package tools_web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newTestDirectProvider(t *testing.T, client *http.Client, maxBytes int64) *directProvider {
	t.Helper()
	return &directProvider{client: client, maxBytes: maxBytes}
}

func TestDirectProvider_ArticleExtraction(t *testing.T) {
	const html = `<!doctype html>
<html><head><title>Test Article</title></head>
<body>
<nav>Home | About | Contact | Login | Signup</nav>
<article>
<h1>Great Article Heading</h1>
<p>This is the first paragraph of substantial content that readability should extract as the main article body text, providing enough words for the extraction algorithm to confidently pick this node as the primary content of the page.</p>
<p>This is the second paragraph continuing the article with more substantive text to ensure the readability parser scores this article node highly relative to the surrounding navigation and footer boilerplate.</p>
</article>
<footer>Copyright 2024 Example Corp. All rights reserved. Privacy Policy | Terms of Service</footer>
</body></html>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(html))
	}))
	t.Cleanup(srv.Close)

	d := newTestDirectProvider(t, srv.Client(), 5<<20)
	page := d.Fetch(context.Background(), srv.URL)

	if !page.OK {
		t.Fatalf("Fetch() OK=false, err=%q, want OK=true", page.Err)
	}
	if !strings.Contains(page.Markdown, "Great Article Heading") {
		t.Errorf("Markdown = %q, want it to contain the article heading", page.Markdown)
	}
	if !strings.Contains(page.Markdown, "first paragraph") {
		t.Errorf("Markdown = %q, want it to contain the article body", page.Markdown)
	}
	if page.Bytes == 0 {
		t.Errorf("Bytes = 0, want a non-zero markdown length")
	}
}

func TestDirectProvider_JSChallenge(t *testing.T) {
	const challengeHTML = `<!doctype html><html><head><title>Just a moment...</title></head>
<body><div class="cf-challenge">Enable JavaScript and cookies to continue</div></body></html>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(challengeHTML))
	}))
	t.Cleanup(srv.Close)

	d := newTestDirectProvider(t, srv.Client(), 5<<20)
	page := d.Fetch(context.Background(), srv.URL)

	if page.OK {
		t.Fatalf("Fetch() OK=true for a JS-challenge page, want OK=false")
	}
	if page.Markdown != "" {
		t.Errorf("Markdown = %q, want empty (no garbage markdown for a challenge page)", page.Markdown)
	}
	if page.Err == "" || !strings.Contains(page.Err, "JS-challenge") {
		t.Errorf("Err = %q, want a clear JS-challenge reason", page.Err)
	}
}

func TestDirectProvider_BodyCap(t *testing.T) {
	// A well-formed but large HTML body; the provider's maxBytes is set far
	// below the actual size so limitedBody must cut the read short.
	var b strings.Builder
	b.WriteString("<html><body><article><h1>Big</h1>")
	for i := 0; i < 100000; i++ {
		b.WriteString("<p>filler paragraph text to inflate body size well past the cap</p>")
	}
	b.WriteString("</article></body></html>")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(b.String()))
	}))
	t.Cleanup(srv.Close)

	d := newTestDirectProvider(t, srv.Client(), 64) // far below body size

	done := make(chan Page, 1)
	go func() { done <- d.Fetch(context.Background(), srv.URL) }()

	select {
	case page := <-done:
		// The body read is capped at 64 bytes regardless of the ~7MB
		// response, so this must return promptly with no OOM. The HTML
		// parser is tolerant of the truncated/malformed tail, so extraction
		// may still succeed on the tiny fragment it did read — what matters
		// is that the output stays bounded to that fragment, not the full
		// response.
		if page.Bytes > 1000 {
			t.Errorf("Bytes = %d, want a small bounded output (body read capped at 64 bytes, not the ~7MB response)", page.Bytes)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Fetch() did not return within 10s — body cap not applied, reading the full oversized body")
	}
}

func TestDirectProvider_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	t.Cleanup(srv.Close)

	d := newTestDirectProvider(t, srv.Client(), 5<<20)

	// A short deadline on the passed-in context is preserved by Fetch's
	// internal context.WithTimeout (it only shortens, never extends, an
	// already-earlier parent deadline), so this stays deterministic and
	// sleepless without touching the package's own 30s constant.
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	page := d.Fetch(ctx, srv.URL)
	elapsed := time.Since(start)

	if page.OK {
		t.Fatalf("Fetch() OK=true for a handler that never responds, want OK=false on timeout")
	}
	if elapsed > 5*time.Second {
		t.Fatalf("Fetch() took %s, want it to return promptly after the context deadline", elapsed)
	}
}

func TestDirectProvider_NilClient(t *testing.T) {
	d := newTestDirectProvider(t, nil, 5<<20)
	page := d.Fetch(context.Background(), "https://example.com")
	if page.OK {
		t.Fatal("Fetch() OK=true with nil client, want OK=false")
	}
	if !strings.Contains(page.Err, "egress not configured") {
		t.Errorf("Err = %q, want it to mention egress not configured", page.Err)
	}
}

func TestDirectProvider_InvalidURL(t *testing.T) {
	d := newTestDirectProvider(t, http.DefaultClient, 5<<20)
	page := d.Fetch(context.Background(), "://not-a-url")
	if page.OK {
		t.Fatal("Fetch() OK=true for an invalid URL, want OK=false")
	}
}

func TestDirectProvider_Name(t *testing.T) {
	d := &directProvider{}
	if d.Name() != "direct" {
		t.Errorf("Name() = %q, want %q", d.Name(), "direct")
	}
}

func TestDetectJSChallenge(t *testing.T) {
	cases := []struct {
		name string
		body string
		want bool
	}{
		{"cloudflare just a moment", "<html>Just a moment...</html>", true},
		{"turnstile marker", `<div class="cf-turnstile"></div>`, true},
		{"clean article", "<html><body><h1>Real content</h1></body></html>", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, hit := detectJSChallenge([]byte(tc.body))
			if hit != tc.want {
				t.Errorf("detectJSChallenge(%q) = %v, want %v", tc.body, hit, tc.want)
			}
		})
	}
}
