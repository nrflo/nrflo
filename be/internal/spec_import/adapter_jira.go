package spec_import

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"be/internal/model"
)

var issueKeyRE = regexp.MustCompile(`^[A-Z][A-Z0-9_]+-[0-9]+$`)

// JiraAdapter fetches Jira issues and searches via the Jira REST API v3.
type JiraAdapter struct {
	client *http.Client
}

func (j *JiraAdapter) Source() Source { return SourceJira }

func (j *JiraAdapter) checkEnv(env map[string]string) (base, email, token string, err error) {
	base = env["JIRA_BASE_URL"]
	email = env["JIRA_EMAIL"]
	token = env["JIRA_API_TOKEN"]
	var missing []string
	if base == "" {
		missing = append(missing, "JIRA_BASE_URL")
	}
	if email == "" {
		missing = append(missing, "JIRA_EMAIL")
	}
	if token == "" {
		missing = append(missing, "JIRA_API_TOKEN")
	}
	if len(missing) > 0 {
		err = MissingEnvError{Source: SourceJira, Missing: missing}
	}
	return
}

func (j *JiraAdapter) parseKey(input string) (string, error) {
	trimmed := strings.TrimSpace(input)
	if issueKeyRE.MatchString(trimmed) {
		return trimmed, nil
	}
	// Try parsing as URL: {base}/browse/{KEY}
	u, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("spec_import(jira): invalid input %q", input)
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	for i, p := range parts {
		if p == "browse" && i+1 < len(parts) {
			key := parts[i+1]
			if issueKeyRE.MatchString(key) {
				return key, nil
			}
		}
	}
	return "", fmt.Errorf("spec_import(jira): cannot parse issue key from %q", input)
}

func (j *JiraAdapter) basicAuth(email, token string) string {
	creds := base64.StdEncoding.EncodeToString([]byte(email + ":" + token))
	return "Basic " + creds
}

// jiraIssueResponse is the minimal shape of the Jira REST API issue response.
type jiraIssueResponse struct {
	Key    string `json:"key"`
	Fields struct {
		Summary     string    `json:"summary"`
		Description *adfNode  `json:"description"`
		Labels      []string  `json:"labels"`
		Components  []struct{ Name string `json:"name"` } `json:"components"`
	} `json:"fields"`
}

func (j *JiraAdapter) Fetch(ctx context.Context, in Input) (FetchedSpec, error) {
	base, email, token, err := j.checkEnv(in.Env)
	if err != nil {
		return FetchedSpec{}, err
	}
	key, err := j.parseKey(in.Body)
	if err != nil {
		return FetchedSpec{}, err
	}

	reqCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	issueURL := strings.TrimRight(base, "/") + "/rest/api/3/issue/" + key
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, issueURL, nil)
	if err != nil {
		return FetchedSpec{}, fmt.Errorf("spec_import(jira): build request: %w", err)
	}
	req.Header.Set("Authorization", j.basicAuth(email, token))
	req.Header.Set("Accept", "application/json")

	resp, err := j.client.Do(req)
	if err != nil {
		return FetchedSpec{}, fmt.Errorf("spec_import(jira): request: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return FetchedSpec{}, ErrAdapterAuth
	case http.StatusNotFound:
		return FetchedSpec{}, ErrAdapterNotFound
	}
	if resp.StatusCode >= 300 {
		return FetchedSpec{}, fmt.Errorf("spec_import(jira): unexpected status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return FetchedSpec{}, fmt.Errorf("spec_import(jira): read body: %w", err)
	}
	var issue jiraIssueResponse
	if err := json.Unmarshal(body, &issue); err != nil {
		return FetchedSpec{}, fmt.Errorf("spec_import(jira): decode: %w", err)
	}

	var sb strings.Builder
	sb.WriteString("# " + issue.Fields.Summary + "\n\n")
	if issue.Fields.Description != nil {
		desc := adfToText(*issue.Fields.Description)
		if desc != "" {
			sb.WriteString(desc + "\n\n")
		}
	}
	if len(issue.Fields.Labels) > 0 {
		sb.WriteString("Labels: " + strings.Join(issue.Fields.Labels, ", ") + "\n")
	}
	if len(issue.Fields.Components) > 0 {
		comps := make([]string, 0, len(issue.Fields.Components))
		for _, c := range issue.Fields.Components {
			comps = append(comps, c.Name)
		}
		sb.WriteString("Components: " + strings.Join(comps, ", ") + "\n")
	}

	browseURL := strings.TrimRight(base, "/") + "/browse/" + key
	ref := model.TicketRef{
		Kind:  string(model.KindSource),
		URL:   browseURL,
		Label: sql.NullString{String: key, Valid: true},
	}

	return FetchedSpec{
		RawText:      strings.TrimSpace(sb.String()),
		AttachedRefs: []model.TicketRef{ref},
	}, nil
}

// JiraIssueSummary is a lightweight result from Search.
type JiraIssueSummary struct {
	Key     string `json:"key"`
	Summary string `json:"summary"`
	Status  string `json:"status"`
}

// Search executes a JQL search and returns up to 20 results.
// If the query looks like an issue key (e.g. PROJ-123) we look it up by key
// instead of running a full-text search, since `text ~` never matches keys.
func (j *JiraAdapter) Search(ctx context.Context, q string, env map[string]string) ([]JiraIssueSummary, error) {
	base, email, token, err := j.checkEnv(env)
	if err != nil {
		return nil, err
	}

	var jql string
	if key := strings.TrimSpace(q); issueKeyRE.MatchString(key) {
		jql = fmt.Sprintf(`key = "%s"`, key)
	} else {
		q = escapeJQL(q)
		if utf8.RuneCountInString(q) > 64 {
			runes := []rune(q)
			q = string(runes[:64])
		}
		jql = fmt.Sprintf(`text ~ "%s" ORDER BY updated DESC`, q)
	}

	payload, _ := json.Marshal(map[string]interface{}{
		"jql":        jql,
		"fields":     []string{"summary", "status", "issuetype", "updated"},
		"maxResults": 20,
	})

	reqCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	searchURL := strings.TrimRight(base, "/") + "/rest/api/3/search/jql"
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, searchURL, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("spec_import(jira): build search request: %w", err)
	}
	req.Header.Set("Authorization", j.basicAuth(email, token))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := j.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("spec_import(jira): search request: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return nil, ErrAdapterAuth
	case http.StatusNotFound:
		return nil, ErrAdapterNotFound
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("spec_import(jira): search status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("spec_import(jira): read search body: %w", err)
	}

	var result struct {
		Issues []struct {
			Key    string `json:"key"`
			Fields struct {
				Summary string `json:"summary"`
				Status  struct {
					Name string `json:"name"`
				} `json:"status"`
			} `json:"fields"`
		} `json:"issues"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("spec_import(jira): decode search: %w", err)
	}

	out := make([]JiraIssueSummary, 0, len(result.Issues))
	for _, iss := range result.Issues {
		out = append(out, JiraIssueSummary{
			Key:     iss.Key,
			Summary: iss.Fields.Summary,
			Status:  iss.Fields.Status.Name,
		})
	}
	return out, nil
}

// escapeJQL sanitizes a query string for embedding in JQL text ~ "..." expressions.
func escapeJQL(q string) string {
	q = strings.ReplaceAll(q, `\`, `\\`)
	q = strings.ReplaceAll(q, `"`, `\"`)
	q = strings.ReplaceAll(q, `*`, ``)
	q = strings.ReplaceAll(q, `?`, ``)
	return q
}
