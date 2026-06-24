package tools_web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestJinaProvider_Fetch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Return-Format") != "markdown" {
			t.Errorf("X-Return-Format = %q", r.Header.Get("X-Return-Format"))
		}
		if !strings.Contains(r.URL.Path, "example.com") {
			t.Errorf("target url not in path: %q", r.URL.Path)
		}
		_, _ = w.Write([]byte("# Title\n\nbody"))
	}))
	defer srv.Close()

	p := &jinaProvider{baseURL: srv.URL, client: srv.Client()}
	page := p.Fetch(context.Background(), "https://example.com/a")
	if !page.OK {
		t.Fatalf("OK = false, err = %q", page.Err)
	}
	if page.Markdown != "# Title\n\nbody" {
		t.Errorf("Markdown = %q", page.Markdown)
	}
	if page.Bytes != len("# Title\n\nbody") {
		t.Errorf("Bytes = %d", page.Bytes)
	}
}

func TestJinaProvider_Non200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	p := &jinaProvider{baseURL: srv.URL, client: srv.Client()}
	page := p.Fetch(context.Background(), "https://blocked.example.com")
	if page.OK {
		t.Fatal("OK = true, want false on 403")
	}
	if page.Err == "" {
		t.Error("Err empty, want a status message")
	}
}

func TestJinaProvider_SetsAuthWhenKeyPresent(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	p := &jinaProvider{baseURL: srv.URL, apiKey: "jk", client: srv.Client()}
	_ = p.Fetch(context.Background(), "https://example.com")
	if gotAuth != "Bearer jk" {
		t.Errorf("Authorization = %q, want %q", gotAuth, "Bearer jk")
	}
}
