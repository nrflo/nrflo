package spec_import

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"be/internal/model"
)

func makeGitHubIssueJSON(number int, title, body string, labels []string) []byte {
	type lbl struct {
		Name string `json:"name"`
	}
	lbls := make([]lbl, len(labels))
	for i, l := range labels {
		lbls[i] = lbl{Name: l}
	}
	v := map[string]interface{}{
		"number": number,
		"title":  title,
		"body":   body,
		"labels": lbls,
	}
	b, _ := json.Marshal(v)
	return b
}

func makeGitHubServer(t *testing.T, issueStatus int, issueBody []byte, commentsBody []byte) (*httptest.Server, *[]string) {
	t.Helper()
	var headers []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers = append(headers, r.Header.Get("Authorization"))
		if strings.Contains(r.URL.Path, "/comments") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			if commentsBody != nil {
				w.Write(commentsBody)
			} else {
				w.Write([]byte("[]"))
			}
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(issueStatus)
		if issueBody != nil {
			w.Write(issueBody)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &headers
}

func githubAdapterWithBase(base string) *GitHubAdapter {
	a := &GitHubAdapter{client: &http.Client{}}
	// patch sharedClient but adapter uses its own field
	_ = base
	return a
}

// issueURLForServer returns a fake GitHub issue URL rooted at srv.
// We override the base URL inside the adapter by monkey-patching the URL.
// Since adapter hardcodes "https://api.github.com", we redirect via transport.
func newGitHubAdapterWithBase(base string) *GitHubAdapter {
	// Use a custom transport that rewrites the host.
	tr := &rewriteTransport{base: base}
	return &GitHubAdapter{client: &http.Client{Transport: tr}}
}

type rewriteTransport struct {
	base string // e.g. "http://127.0.0.1:PORT"
}

func (rt *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.URL.Scheme = "http"
	clone.URL.Host = strings.TrimPrefix(strings.TrimPrefix(rt.base, "https://"), "http://")
	return http.DefaultTransport.RoundTrip(clone)
}

func TestGitHubAdapter_Fetch_Success(t *testing.T) {
	issueJSON := makeGitHubIssueJSON(42, "My Issue", "Issue body text", []string{"bug"})
	commentsJSON := []byte(`[{"user":{"login":"alice"},"body":"first comment"}]`)
	srv, _ := makeGitHubServer(t, http.StatusOK, issueJSON, commentsJSON)

	a := newGitHubAdapterWithBase(srv.URL)
	in := Input{
		Body: "https://github.com/owner/repo/issues/42",
		Env:  map[string]string{"GITHUB_TOKEN": "tok-123"},
	}
	spec, err := a.Fetch(context.Background(), in)
	if err != nil {
		t.Fatalf("Fetch error: %v", err)
	}
	if spec.RawText == "" {
		t.Error("RawText is empty")
	}
	if !strings.Contains(spec.RawText, "My Issue") {
		t.Errorf("RawText missing title: %q", spec.RawText)
	}
	if !strings.Contains(spec.RawText, "Issue body text") {
		t.Errorf("RawText missing body: %q", spec.RawText)
	}
	if !strings.Contains(spec.RawText, "first comment") {
		t.Errorf("RawText missing comment: %q", spec.RawText)
	}

	if len(spec.AttachedRefs) != 1 {
		t.Fatalf("AttachedRefs len = %d, want 1", len(spec.AttachedRefs))
	}
	ref := spec.AttachedRefs[0]
	if ref.Kind != string(model.KindSource) {
		t.Errorf("ref.Kind = %q, want %q", ref.Kind, model.KindSource)
	}
	if ref.Label.String != "owner/repo#42" {
		t.Errorf("ref.Label = %q, want owner/repo#42", ref.Label.String)
	}
	if ref.URL != in.Body {
		t.Errorf("ref.URL = %q, want %q", ref.URL, in.Body)
	}
}

func TestGitHubAdapter_Fetch_EmptyComments(t *testing.T) {
	issueJSON := makeGitHubIssueJSON(1, "Title Only", "body", nil)
	srv, _ := makeGitHubServer(t, http.StatusOK, issueJSON, []byte("[]"))

	a := newGitHubAdapterWithBase(srv.URL)
	in := Input{Body: "https://github.com/o/r/issues/1", Env: map[string]string{}}
	spec, err := a.Fetch(context.Background(), in)
	if err != nil {
		t.Fatalf("Fetch error: %v", err)
	}
	if spec.RawText == "" {
		t.Error("RawText is empty")
	}
	if len(spec.AttachedRefs) != 1 {
		t.Errorf("AttachedRefs len = %d, want 1", len(spec.AttachedRefs))
	}
}

func TestGitHubAdapter_Fetch_NoToken_NoAuthHeader(t *testing.T) {
	issueJSON := makeGitHubIssueJSON(7, "Public Issue", "", nil)
	var capturedAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		if strings.Contains(r.URL.Path, "/comments") {
			w.Write([]byte("[]"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(issueJSON)
	}))
	t.Cleanup(srv.Close)

	a := newGitHubAdapterWithBase(srv.URL)
	in := Input{Body: "https://github.com/pub/repo/issues/7", Env: map[string]string{}}
	_, err := a.Fetch(context.Background(), in)
	if err != nil {
		t.Fatalf("Fetch error: %v", err)
	}
	if capturedAuth != "" {
		t.Errorf("Authorization header sent without token: %q", capturedAuth)
	}
}

func TestGitHubAdapter_Fetch_WithToken_BearerHeader(t *testing.T) {
	issueJSON := makeGitHubIssueJSON(3, "T", "", nil)
	var capturedAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		if strings.Contains(r.URL.Path, "/comments") {
			w.Write([]byte("[]"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(issueJSON)
	}))
	t.Cleanup(srv.Close)

	a := newGitHubAdapterWithBase(srv.URL)
	in := Input{Body: "https://github.com/o/r/issues/3", Env: map[string]string{"GITHUB_TOKEN": "mytoken"}}
	_, err := a.Fetch(context.Background(), in)
	if err != nil {
		t.Fatalf("Fetch error: %v", err)
	}
	if capturedAuth != "Bearer mytoken" {
		t.Errorf("Authorization = %q, want %q", capturedAuth, "Bearer mytoken")
	}
}

func TestGitHubAdapter_Fetch_401(t *testing.T) {
	srv, _ := makeGitHubServer(t, http.StatusUnauthorized, nil, nil)
	a := newGitHubAdapterWithBase(srv.URL)
	in := Input{Body: "https://github.com/o/r/issues/1", Env: map[string]string{}}
	_, err := a.Fetch(context.Background(), in)
	if !errors.Is(err, ErrAdapterAuth) {
		t.Errorf("err = %v, want ErrAdapterAuth", err)
	}
}

func TestGitHubAdapter_Fetch_404(t *testing.T) {
	srv, _ := makeGitHubServer(t, http.StatusNotFound, nil, nil)
	a := newGitHubAdapterWithBase(srv.URL)
	in := Input{Body: "https://github.com/o/r/issues/1", Env: map[string]string{}}
	_, err := a.Fetch(context.Background(), in)
	if !errors.Is(err, ErrAdapterNotFound) {
		t.Errorf("err = %v, want ErrAdapterNotFound", err)
	}
}

func TestParseGitHubURL_RejectsPullRequest(t *testing.T) {
	_, _, _, err := parseGitHubURL("https://github.com/owner/repo/pull/5")
	if err == nil {
		t.Error("expected error for pull request URL")
	}
	if !strings.Contains(err.Error(), "pull request") {
		t.Errorf("error %q should mention pull request", err.Error())
	}
}

func TestParseGitHubURL_RejectsMalformed(t *testing.T) {
	cases := []string{
		"not-a-url",
		"https://github.com/owner",
		"https://github.com/owner/repo/wiki/42",
	}
	for _, c := range cases {
		_, _, _, err := parseGitHubURL(c)
		if err == nil {
			t.Errorf("parseGitHubURL(%q) expected error, got nil", c)
		}
	}
}

func TestGitHubAdapter_Search_MissingToken(t *testing.T) {
	a := &GitHubAdapter{client: &http.Client{}}
	_, err := a.Search(context.Background(), "query", "", map[string]string{})
	var me MissingEnvError
	if !errors.As(err, &me) {
		t.Fatalf("err = %v, want MissingEnvError", err)
	}
	if len(me.Missing) != 1 || me.Missing[0] != "GITHUB_TOKEN" {
		t.Errorf("Missing = %v, want [GITHUB_TOKEN]", me.Missing)
	}
}

func TestGitHubAdapter_Search_Success(t *testing.T) {
	respJSON := `{"items":[{"number":10,"title":"found","html_url":"https://github.com/o/r/issues/10","state":"open","updated_at":"2024-01-01"}]}`
	var capturedAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(respJSON))
	}))
	t.Cleanup(srv.Close)

	a := newGitHubAdapterWithBase(srv.URL)
	results, err := a.Search(context.Background(), "found", "", map[string]string{"GITHUB_TOKEN": "tok"})
	if err != nil {
		t.Fatalf("Search error: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("results len = %d, want 1", len(results))
	}
	if capturedAuth != "Bearer tok" {
		t.Errorf("Authorization = %q, want %q", capturedAuth, "Bearer tok")
	}
}
