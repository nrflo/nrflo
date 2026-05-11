package spec_import

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"be/internal/model"
)

// GitHubAdapter fetches GitHub issues and searches via the GitHub REST API.
type GitHubAdapter struct {
	client *http.Client
}

func (g *GitHubAdapter) Source() Source { return SourceGitHubIssue }

type githubIssue struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	Body   string `json:"body"`
	Labels []struct {
		Name string `json:"name"`
	} `json:"labels"`
}

type githubComment struct {
	User struct {
		Login string `json:"login"`
	} `json:"user"`
	Body string `json:"body"`
}

// GitHubIssueSummary is a lightweight result from Search.
type GitHubIssueSummary struct {
	Number    int    `json:"number"`
	Title     string `json:"title"`
	HTMLURL   string `json:"html_url"`
	State     string `json:"state"`
	UpdatedAt string `json:"updated_at"`
}

// parseGitHubURL extracts owner, repo, issue number from an issue URL or
// returns an error for pull request URLs or malformed paths.
func parseGitHubURL(raw string) (owner, repo string, number int, err error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", "", 0, fmt.Errorf("spec_import(github): invalid URL: %w", err)
	}
	// Expect path like /owner/repo/issues/N
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 4 {
		return "", "", 0, fmt.Errorf("spec_import(github): path too short: %q", u.Path)
	}
	owner = parts[0]
	repo = parts[1]
	segment := parts[2]
	nStr := parts[3]

	if segment == "pull" || segment == "pulls" {
		return "", "", 0, fmt.Errorf("spec_import(github): pull request URLs are not supported")
	}
	if segment != "issues" {
		return "", "", 0, fmt.Errorf("spec_import(github): expected /issues/ segment, got %q", segment)
	}
	n, err := strconv.Atoi(nStr)
	if err != nil || n <= 0 {
		return "", "", 0, fmt.Errorf("spec_import(github): invalid issue number %q", nStr)
	}
	return owner, repo, n, nil
}

func (g *GitHubAdapter) authHeader(env map[string]string) string {
	if t := env["GITHUB_TOKEN"]; t != "" {
		return "Bearer " + t
	}
	return ""
}

func (g *GitHubAdapter) doGet(ctx context.Context, rawURL string, auth string) (*http.Response, error) {
	reqCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	return g.client.Do(req)
}

func (g *GitHubAdapter) Fetch(ctx context.Context, in Input) (FetchedSpec, error) {
	owner, repo, number, err := parseGitHubURL(in.Body)
	if err != nil {
		return FetchedSpec{}, err
	}
	auth := g.authHeader(in.Env)
	base := "https://api.github.com"

	// Fetch issue.
	issueURL := fmt.Sprintf("%s/repos/%s/%s/issues/%d", base, owner, repo, number)
	resp, err := g.doGet(ctx, issueURL, auth)
	if err != nil {
		return FetchedSpec{}, fmt.Errorf("spec_import(github): issue request: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return FetchedSpec{}, ErrAdapterAuth
	case http.StatusNotFound:
		return FetchedSpec{}, ErrAdapterNotFound
	}
	if resp.StatusCode >= 300 {
		return FetchedSpec{}, fmt.Errorf("spec_import(github): unexpected status %d", resp.StatusCode)
	}

	issueBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return FetchedSpec{}, fmt.Errorf("spec_import(github): read issue body: %w", err)
	}
	var issue githubIssue
	if err := json.Unmarshal(issueBody, &issue); err != nil {
		return FetchedSpec{}, fmt.Errorf("spec_import(github): decode issue: %w", err)
	}

	// Fetch comments.
	commentsURL := fmt.Sprintf("%s/repos/%s/%s/issues/%d/comments?per_page=20", base, owner, repo, number)
	cResp, err := g.doGet(ctx, commentsURL, auth)
	if err != nil {
		return FetchedSpec{}, fmt.Errorf("spec_import(github): comments request: %w", err)
	}
	defer cResp.Body.Close()
	var comments []githubComment
	if cResp.StatusCode == http.StatusOK {
		cBody, err := io.ReadAll(io.LimitReader(cResp.Body, 1<<20))
		if err != nil {
			return FetchedSpec{}, fmt.Errorf("spec_import(github): read comments body: %w", err)
		}
		_ = json.Unmarshal(cBody, &comments)
	}

	// Build RawText.
	var sb strings.Builder
	sb.WriteString("# " + issue.Title + "\n\n")
	if issue.Body != "" {
		sb.WriteString(issue.Body + "\n\n")
	}
	if len(issue.Labels) > 0 {
		names := make([]string, 0, len(issue.Labels))
		for _, l := range issue.Labels {
			names = append(names, l.Name)
		}
		sb.WriteString("Labels: " + strings.Join(names, ", ") + "\n\n")
	}
	for _, c := range comments {
		sb.WriteString(fmt.Sprintf("**%s**: %s\n\n", c.User.Login, c.Body))
	}

	label := fmt.Sprintf("%s/%s#%d", owner, repo, number)
	ref := model.TicketRef{
		Kind:  string(model.KindSource),
		URL:   strings.TrimSpace(in.Body),
		Label: sql.NullString{String: label, Valid: true},
	}

	return FetchedSpec{
		RawText:      strings.TrimSpace(sb.String()),
		AttachedRefs: []model.TicketRef{ref},
	}, nil
}

// Search queries GitHub Issues matching q, optionally scoped to owner/repo.
// Returns MissingEnvError when GITHUB_TOKEN is absent.
func (g *GitHubAdapter) Search(ctx context.Context, q, ownerRepo string, env map[string]string) ([]GitHubIssueSummary, error) {
	token := env["GITHUB_TOKEN"]
	if token == "" {
		return nil, MissingEnvError{Source: SourceGitHubIssue, Missing: []string{"GITHUB_TOKEN"}}
	}
	auth := "Bearer " + token

	parts := []string{"is:issue", q}
	if ownerRepo != "" {
		parts = append(parts, "repo:"+ownerRepo)
	}
	qStr := strings.Join(parts, " ")

	searchURL := "https://api.github.com/search/issues?q=" + url.QueryEscape(qStr) + "&sort=updated&order=desc&per_page=20"

	reqCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, searchURL, nil)
	if err != nil {
		return nil, fmt.Errorf("spec_import(github): build search request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", auth)

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("spec_import(github): search request: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return nil, ErrAdapterAuth
	case http.StatusNotFound:
		return nil, ErrAdapterNotFound
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("spec_import(github): search status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("spec_import(github): read search body: %w", err)
	}

	var result struct {
		Items []GitHubIssueSummary `json:"items"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("spec_import(github): decode search: %w", err)
	}
	return result.Items, nil
}
