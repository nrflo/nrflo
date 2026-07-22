package ollamanative

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestRun_AuthorizationHeader verifies Run only sends a Bearer Authorization
// header when the credential value is non-empty (local servers with no auth
// need no header at all).
func TestRun_AuthorizationHeader(t *testing.T) {
	t.Run("empty api key omits header", func(t *testing.T) {
		var gotAuth, gotPath string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotAuth = r.Header.Get("Authorization")
			gotPath = r.URL.Path
			w.Header().Set("Content-Type", "application/x-ndjson")
			_, _ = w.Write([]byte(`{"message":{"content":"ok"},"done":true,"done_reason":"stop"}` + "\n"))
		}))
		t.Cleanup(srv.Close)

		p := New(Credentials{BaseURL: srv.URL})
		if _, err := p.Run(context.Background(), minimalRequest(), &recordingSink{}); err != nil {
			t.Fatalf("Run: %v", err)
		}
		if gotPath != "/api/chat" {
			t.Errorf("path = %q, want /api/chat", gotPath)
		}
		if gotAuth != "" {
			t.Errorf("Authorization = %q, want empty (no api key)", gotAuth)
		}
	})

	t.Run("non-empty api key sends Bearer header", func(t *testing.T) {
		var gotAuth string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotAuth = r.Header.Get("Authorization")
			w.Header().Set("Content-Type", "application/x-ndjson")
			_, _ = w.Write([]byte(`{"message":{"content":"ok"},"done":true,"done_reason":"stop"}` + "\n"))
		}))
		t.Cleanup(srv.Close)

		p := New(Credentials{BaseURL: srv.URL, Value: "sk-local-test"})
		if _, err := p.Run(context.Background(), minimalRequest(), &recordingSink{}); err != nil {
			t.Fatalf("Run: %v", err)
		}
		if gotAuth != "Bearer sk-local-test" {
			t.Errorf("Authorization = %q, want %q", gotAuth, "Bearer sk-local-test")
		}
	})
}

// TestRun_NonOKStatus verifies a non-2xx response from /api/chat surfaces a
// descriptive error rather than being decoded as a stream.
func TestRun_NonOKStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	p := New(Credentials{BaseURL: srv.URL})
	_, err := p.Run(context.Background(), minimalRequest(), &recordingSink{})
	if err == nil {
		t.Fatal("Run succeeded, want error for 500 status")
	}
}

// TestName_MaxContext verifies the provider's identity/flat default context.
func TestName_MaxContext(t *testing.T) {
	p := New(Credentials{BaseURL: "http://localhost:11434"})
	if p.Name() != "ollamanative" {
		t.Errorf("Name() = %q, want ollamanative", p.Name())
	}
	if got := p.MaxContext("anything"); got != 128000 {
		t.Errorf("MaxContext = %d, want 128000", got)
	}
}
