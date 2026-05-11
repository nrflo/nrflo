package spec_import

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"errors"
)

func TestJiraAdapter_Search_MissingEnv(t *testing.T) {
	var hitCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitCount++
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	a := &JiraAdapter{client: &http.Client{}}
	hitCount = 0
	_, err := a.Search(context.Background(), "query", map[string]string{"JIRA_BASE_URL": srv.URL, "JIRA_EMAIL": "e"})
	var me MissingEnvError
	if !errors.As(err, &me) {
		t.Fatalf("err = %v, want MissingEnvError", err)
	}
	if hitCount != 0 {
		t.Errorf("server hit = %d, want 0", hitCount)
	}
}

func TestJiraAdapter_Search_JQLEscaping(t *testing.T) {
	var capturedBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedBody, _ = readAll1MiB(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"issues":[]}`))
	}))
	t.Cleanup(srv.Close)

	a := &JiraAdapter{client: &http.Client{}}
	_, err := a.Search(context.Background(), `foo"bar\baz`, fullJiraEnv(srv.URL))
	if err != nil {
		t.Fatalf("Search error: %v", err)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(capturedBody, &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	jql, _ := payload["jql"].(string)
	// quotes and backslashes should be escaped in the JQL
	if strings.Contains(jql, `"bar"`) {
		t.Errorf("JQL quote not escaped: %q", jql)
	}
}

func TestJiraAdapter_Search_QueryCapped64(t *testing.T) {
	var capturedBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedBody, _ = readAll1MiB(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"issues":[]}`))
	}))
	t.Cleanup(srv.Close)

	longQ := strings.Repeat("a", 100)
	a := &JiraAdapter{client: &http.Client{}}
	_, err := a.Search(context.Background(), longQ, fullJiraEnv(srv.URL))
	if err != nil {
		t.Fatalf("Search error: %v", err)
	}

	var payload map[string]interface{}
	json.Unmarshal(capturedBody, &payload)
	jql, _ := payload["jql"].(string)
	// The embedded query between quotes should be at most 64 runes.
	start := strings.Index(jql, `"`)
	end := strings.LastIndex(jql, `"`)
	if start >= 0 && end > start {
		embedded := jql[start+1 : end]
		if len([]rune(embedded)) > 64 {
			t.Errorf("embedded query rune count = %d, want ≤64", len([]rune(embedded)))
		}
	}
}

// readAll1MiB reads all bytes from r (up to buffer limits).
func readAll1MiB(r interface{ Read([]byte) (int, error) }) ([]byte, error) {
	buf := make([]byte, 0, 512)
	tmp := make([]byte, 512)
	for {
		n, err := r.Read(tmp)
		buf = append(buf, tmp[:n]...)
		if err != nil {
			break
		}
	}
	return buf, nil
}
