package tools_web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

const ddgFixture = `<html><body>
<div class="result">
  <a rel="nofollow" class="result__a" href="//duckduckgo.com/l/?uddg=https%3A%2F%2Fexample.com%2Fdocs&amp;rut=abc">Example <b>Docs</b></a>
  <a class="result__snippet" href="#">Snippet <b>one</b> text</a>
</div>
<div class="result">
  <a rel="nofollow" class="result__a" href="https://direct.example.org/page">Direct Result</a>
  <a class="result__snippet" href="#">Snippet two</a>
</div>
<div class="result">
  <a rel="nofollow" class="result__a" href="https://duckduckgo.com/y.js?ad=1">Ad dropped</a>
</div>
</body></html>`

func TestDDGProvider_SearchParsesAndDecodes(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_, _ = w.Write([]byte(ddgFixture))
	}))
	t.Cleanup(srv.Close)

	p := &ddgProvider{baseURL: srv.URL, client: srv.Client()}
	results, err := p.Search(context.Background(), "golang ssrf", SearchOpts{MaxResults: 5})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if gotQuery != "q=golang+ssrf" {
		t.Errorf("query = %q", gotQuery)
	}
	if len(results) != 2 {
		t.Fatalf("results = %+v, want 2 (ad dropped)", results)
	}
	if results[0].URL != "https://example.com/docs" || results[0].Title != "Example Docs" || results[0].Snippet != "Snippet one text" {
		t.Errorf("first = %+v", results[0])
	}
	if results[1].URL != "https://direct.example.org/page" || results[1].Title != "Direct Result" {
		t.Errorf("second = %+v", results[1])
	}
}

func TestDDGProvider_MaxResultsCap(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(ddgFixture))
	}))
	t.Cleanup(srv.Close)

	p := &ddgProvider{baseURL: srv.URL, client: srv.Client()}
	results, err := p.Search(context.Background(), "q", SearchOpts{MaxResults: 1})
	if err != nil || len(results) != 1 {
		t.Fatalf("got %d results, %v; want 1", len(results), err)
	}
}

func TestDDGProvider_NonOKStatusErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(srv.Close)

	p := &ddgProvider{baseURL: srv.URL, client: srv.Client()}
	if _, err := p.Search(context.Background(), "q", SearchOpts{}); err == nil {
		t.Fatal("want error on 403")
	}
}

func TestDDGDecodeHref(t *testing.T) {
	cases := []struct{ in, want string }{
		{"//duckduckgo.com/l/?uddg=https%3A%2F%2Fa.example%2Fx%3Fy%3D1&rut=z", "https://a.example/x?y=1"},
		{"https://direct.example.org/p", "https://direct.example.org/p"},
		{"https://duckduckgo.com/y.js?ad=1", ""},
		{"/internal/nav", ""},
	}
	for _, c := range cases {
		if got := ddgDecodeHref(c.in); got != c.want {
			t.Errorf("ddgDecodeHref(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
