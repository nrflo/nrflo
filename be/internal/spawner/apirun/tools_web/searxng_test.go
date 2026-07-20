package tools_web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSearxngProvider_Search(t *testing.T) {
	var gotPath, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"results": [
				{"title": "First", "url": "https://a.example.com", "content": "snippet a", "score": 1.5, "engine": "google"},
				{"title": "Second", "url": "https://b.example.com", "content": "snippet b", "score": 0.9, "engine": "bing"},
				{"title": "no url, skipped", "url": "", "content": "x", "score": 0.1, "engine": "x"}
			]
		}`))
	}))
	t.Cleanup(srv.Close)

	p := &searxngProvider{baseURL: srv.URL, client: srv.Client()}
	results, err := p.Search(context.Background(), "golang ssrf", SearchOpts{MaxResults: 5})
	if err != nil {
		t.Fatalf("Search() unexpected err: %v", err)
	}

	if gotPath != "/search" {
		t.Errorf("path = %q, want /search", gotPath)
	}
	if !strings.Contains(gotQuery, "format=json") {
		t.Errorf("query = %q, want it to contain format=json", gotQuery)
	}
	if !strings.Contains(gotQuery, "q=golang") {
		t.Errorf("query = %q, want it to contain q=golang", gotQuery)
	}

	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2 (empty-URL result skipped)", len(results))
	}
	if results[0].URL != "https://a.example.com" || results[0].Title != "First" || results[0].Snippet != "snippet a" || results[0].Score != 1.5 {
		t.Errorf("results[0] = %+v, unexpected", results[0])
	}
	if results[1].URL != "https://b.example.com" || results[1].Snippet != "snippet b" {
		t.Errorf("results[1] = %+v, unexpected", results[1])
	}
}

func TestSearxngProvider_MaxResultsCap(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results": [
			{"title":"1","url":"https://a.example.com","content":"x"},
			{"title":"2","url":"https://b.example.com","content":"x"},
			{"title":"3","url":"https://c.example.com","content":"x"}
		]}`))
	}))
	t.Cleanup(srv.Close)

	p := &searxngProvider{baseURL: srv.URL, client: srv.Client()}
	results, err := p.Search(context.Background(), "q", SearchOpts{MaxResults: 2})
	if err != nil {
		t.Fatalf("Search() unexpected err: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2 (MaxResults cap)", len(results))
	}
}

func TestSearxngProvider_Forbidden(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(srv.Close)

	p := &searxngProvider{baseURL: srv.URL, client: srv.Client()}
	_, err := p.Search(context.Background(), "q", SearchOpts{})
	if err == nil {
		t.Fatal("Search() err = nil, want an actionable error on 403")
	}
	if !strings.Contains(err.Error(), "formats: [html, json]") {
		t.Errorf("err = %q, want it to name `formats: [html, json]`", err.Error())
	}
}

func TestSearxngProvider_OtherStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("boom"))
	}))
	t.Cleanup(srv.Close)

	p := &searxngProvider{baseURL: srv.URL, client: srv.Client()}
	_, err := p.Search(context.Background(), "q", SearchOpts{})
	if err == nil {
		t.Fatal("Search() err = nil, want error on 500")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("err = %q, want it to mention status 500", err.Error())
	}
}

func TestSearxngProvider_MissingBaseURL(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
	}))
	t.Cleanup(srv.Close)

	p := &searxngProvider{baseURL: "", client: srv.Client()}
	_, err := p.Search(context.Background(), "q", SearchOpts{})
	if err == nil {
		t.Fatal("Search() err = nil, want error when SEARXNG_BASE_URL unset")
	}
	if !strings.Contains(err.Error(), "SEARXNG_BASE_URL") {
		t.Errorf("err = %q, want it to mention SEARXNG_BASE_URL", err.Error())
	}
	if calls != 0 {
		t.Errorf("calls = %d, want 0 (no HTTP call without a base URL)", calls)
	}
}

func TestSearxngProvider_NilClient(t *testing.T) {
	p := &searxngProvider{baseURL: "http://example.com", client: nil}
	_, err := p.Search(context.Background(), "q", SearchOpts{})
	if err == nil {
		t.Fatal("Search() err = nil, want error when egress not configured")
	}
}

func TestSearxngProvider_DecodeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("not json"))
	}))
	t.Cleanup(srv.Close)

	p := &searxngProvider{baseURL: srv.URL, client: srv.Client()}
	_, err := p.Search(context.Background(), "q", SearchOpts{})
	if err == nil {
		t.Fatal("Search() err = nil, want decode error on malformed JSON")
	}
}

func TestSearxngProvider_Name(t *testing.T) {
	p := &searxngProvider{}
	if p.Name() != "searxng" {
		t.Errorf("Name() = %q, want %q", p.Name(), "searxng")
	}
}
