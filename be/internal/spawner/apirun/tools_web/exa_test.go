package tools_web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestExaProvider_Search(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search" {
			t.Errorf("path = %q, want /search", r.URL.Path)
		}
		if r.Header.Get("x-api-key") != "secret" {
			t.Errorf("missing/x-api-key header = %q", r.Header.Get("x-api-key"))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]any{
				{"url": "https://a.com", "title": "A", "text": "snippet-a", "score": 0.9},
				{"url": "", "title": "skip-me"}, // empty URL dropped
				{"url": "https://b.com", "title": "B", "score": 0.5},
			},
		})
	}))
	defer srv.Close()

	p := &exaProvider{baseURL: srv.URL, apiKey: "secret", client: srv.Client()}
	res, err := p.Search(context.Background(), "q", SearchOpts{MaxResults: 5})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(res) != 2 {
		t.Fatalf("results = %d, want 2 (empty url dropped)", len(res))
	}
	if res[0].URL != "https://a.com" || res[0].Snippet != "snippet-a" || res[0].Score != 0.9 {
		t.Errorf("res[0] = %+v", res[0])
	}
}

func TestExaProvider_NoKey(t *testing.T) {
	p := &exaProvider{baseURL: "https://unused", apiKey: "", client: http.DefaultClient}
	if _, err := p.Search(context.Background(), "q", SearchOpts{}); err == nil {
		t.Fatal("expected error when EXA_API_KEY unset")
	}
}

func TestExaProvider_Non200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("nope"))
	}))
	defer srv.Close()
	p := &exaProvider{baseURL: srv.URL, apiKey: "k", client: srv.Client()}
	if _, err := p.Search(context.Background(), "q", SearchOpts{}); err == nil {
		t.Fatal("expected error on non-200")
	}
}
