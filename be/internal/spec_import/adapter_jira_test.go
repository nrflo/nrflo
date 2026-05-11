package spec_import

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func fullJiraEnv(base string) map[string]string {
	return map[string]string{
		"JIRA_BASE_URL":  base,
		"JIRA_EMAIL":     "user@example.com",
		"JIRA_API_TOKEN": "tok",
	}
}

func makeJiraIssueResponse(key, summary string, desc *adfNode, labels []string) []byte {
	type fields struct {
		Summary     string   `json:"summary"`
		Description *adfNode `json:"description"`
		Labels      []string `json:"labels"`
	}
	v := struct {
		Key    string `json:"key"`
		Fields fields `json:"fields"`
	}{Key: key, Fields: fields{Summary: summary, Description: desc, Labels: labels}}
	b, _ := json.Marshal(v)
	return b
}

func TestJiraAdapter_Fetch_Success(t *testing.T) {
	desc := &adfNode{
		Type: "doc",
		Content: []adfNode{
			{Type: "paragraph", Content: []adfNode{{Type: "text", Text: "Hello world"}}},
		},
	}
	issueJSON := makeJiraIssueResponse("PROJ-123", "Test issue", desc, []string{"backend"})

	var capturedAuth string
	var hitCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitCount++
		capturedAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(issueJSON)
	}))
	t.Cleanup(srv.Close)

	a := &JiraAdapter{client: &http.Client{}}
	in := Input{Body: "PROJ-123", Env: fullJiraEnv(srv.URL)}
	spec, err := a.Fetch(context.Background(), in)
	if err != nil {
		t.Fatalf("Fetch error: %v", err)
	}
	if spec.RawText == "" {
		t.Error("RawText is empty")
	}
	if !strings.Contains(spec.RawText, "Test issue") {
		t.Errorf("RawText missing summary: %q", spec.RawText)
	}
	if !strings.Contains(spec.RawText, "Hello world") {
		t.Errorf("RawText missing description: %q", spec.RawText)
	}
	if !strings.Contains(spec.RawText, "backend") {
		t.Errorf("RawText missing label: %q", spec.RawText)
	}

	// basic auth
	wantAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("user@example.com:tok"))
	if capturedAuth != wantAuth {
		t.Errorf("Authorization = %q, want %q", capturedAuth, wantAuth)
	}

	if len(spec.AttachedRefs) != 1 {
		t.Fatalf("AttachedRefs len = %d, want 1", len(spec.AttachedRefs))
	}
	ref := spec.AttachedRefs[0]
	if ref.Kind != "source" {
		t.Errorf("ref.Kind = %q, want source", ref.Kind)
	}
	if ref.Label.String != "PROJ-123" {
		t.Errorf("ref.Label = %q, want PROJ-123", ref.Label.String)
	}
	if !strings.Contains(ref.URL, "PROJ-123") {
		t.Errorf("ref.URL = %q, want to contain PROJ-123", ref.URL)
	}
}

func TestJiraAdapter_Fetch_KeyFromBrowseURL(t *testing.T) {
	issueJSON := makeJiraIssueResponse("ACME-7", "Browse test", nil, nil)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "ACME-7") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(issueJSON)
	}))
	t.Cleanup(srv.Close)

	a := &JiraAdapter{client: &http.Client{}}
	in := Input{
		Body: srv.URL + "/browse/ACME-7",
		Env:  fullJiraEnv(srv.URL),
	}
	spec, err := a.Fetch(context.Background(), in)
	if err != nil {
		t.Fatalf("Fetch error: %v", err)
	}
	if len(spec.AttachedRefs) != 1 || spec.AttachedRefs[0].Label.String != "ACME-7" {
		t.Errorf("unexpected ref: %+v", spec.AttachedRefs)
	}
}

func TestJiraAdapter_Fetch_MissingEnv(t *testing.T) {
	var hitCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitCount++
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	cases := []struct {
		name    string
		env     map[string]string
		missing string
	}{
		{"no base", map[string]string{"JIRA_EMAIL": "e", "JIRA_API_TOKEN": "t"}, "JIRA_BASE_URL"},
		{"no email", map[string]string{"JIRA_BASE_URL": srv.URL, "JIRA_API_TOKEN": "t"}, "JIRA_EMAIL"},
		{"no token", map[string]string{"JIRA_BASE_URL": srv.URL, "JIRA_EMAIL": "e"}, "JIRA_API_TOKEN"},
	}

	a := &JiraAdapter{client: &http.Client{}}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hitCount = 0
			_, err := a.Fetch(context.Background(), Input{Body: "PROJ-1", Env: tc.env})
			var me MissingEnvError
			if !errors.As(err, &me) {
				t.Fatalf("err = %v, want MissingEnvError", err)
			}
			found := false
			for _, m := range me.Missing {
				if m == tc.missing {
					found = true
				}
			}
			if !found {
				t.Errorf("Missing = %v, want to contain %q", me.Missing, tc.missing)
			}
			if hitCount != 0 {
				t.Errorf("server hit count = %d, want 0 (no network call on missing env)", hitCount)
			}
		})
	}
}

func TestJiraAdapter_Fetch_401(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(srv.Close)

	a := &JiraAdapter{client: &http.Client{}}
	_, err := a.Fetch(context.Background(), Input{Body: "PROJ-1", Env: fullJiraEnv(srv.URL)})
	if !errors.Is(err, ErrAdapterAuth) {
		t.Errorf("err = %v, want ErrAdapterAuth", err)
	}
}

func TestJiraAdapter_Fetch_404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	a := &JiraAdapter{client: &http.Client{}}
	_, err := a.Fetch(context.Background(), Input{Body: "PROJ-1", Env: fullJiraEnv(srv.URL)})
	if !errors.Is(err, ErrAdapterNotFound) {
		t.Errorf("err = %v, want ErrAdapterNotFound", err)
	}
}

func TestJiraAdapter_Fetch_MissingDescription(t *testing.T) {
	issueJSON := makeJiraIssueResponse("TEAM-5", "No desc issue", nil, []string{"label1"})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(issueJSON)
	}))
	t.Cleanup(srv.Close)

	a := &JiraAdapter{client: &http.Client{}}
	spec, err := a.Fetch(context.Background(), Input{Body: "TEAM-5", Env: fullJiraEnv(srv.URL)})
	if err != nil {
		t.Fatalf("Fetch error: %v", err)
	}
	if spec.RawText == "" {
		t.Error("RawText is empty")
	}
	if !strings.Contains(spec.RawText, "No desc issue") {
		t.Errorf("RawText missing summary: %q", spec.RawText)
	}
	if !strings.Contains(spec.RawText, "label1") {
		t.Errorf("RawText missing label: %q", spec.RawText)
	}
	if len(spec.AttachedRefs) != 1 {
		t.Errorf("AttachedRefs len = %d, want 1", len(spec.AttachedRefs))
	}
}

