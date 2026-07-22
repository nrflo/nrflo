package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestCheckConnection_HappyPath verifies a valid OpenAI-compatible /models
// response is decoded into a flat list of model ids.
func TestCheckConnection_HappyPath(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			t.Errorf("path = %q, want /models", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]string{{"id": "qwen3.5:4b"}},
		})
	}))
	t.Cleanup(srv.Close)

	svc := setupCustomProviderService(t)
	ids, err := svc.CheckConnection(srv.URL, "", "")
	if err != nil {
		t.Fatalf("CheckConnection: %v", err)
	}
	if len(ids) != 1 || ids[0] != "qwen3.5:4b" {
		t.Errorf("ids = %v, want [qwen3.5:4b]", ids)
	}
}

// TestCheckConnection_NonOKStatus verifies non-2xx responses surface a
// descriptive error rather than being decoded as success.
func TestCheckConnection_NonOKStatus(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		status int
	}{
		{"500", http.StatusInternalServerError},
		{"404", http.StatusNotFound},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
			}))
			t.Cleanup(srv.Close)

			svc := setupCustomProviderService(t)
			_, err := svc.CheckConnection(srv.URL, "", "")
			if err == nil {
				t.Fatal("CheckConnection succeeded, want error")
			}
			if !strings.Contains(err.Error(), "status") {
				t.Errorf("error = %v, want mention of status", err)
			}
		})
	}
}

// TestCheckConnection_InvalidJSONBody verifies a malformed response body
// produces a descriptive decode error.
func TestCheckConnection_InvalidJSONBody(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("not json"))
	}))
	t.Cleanup(srv.Close)

	svc := setupCustomProviderService(t)
	_, err := svc.CheckConnection(srv.URL, "", "")
	if err == nil {
		t.Fatal("CheckConnection succeeded, want error")
	}
	if !strings.Contains(err.Error(), "decode") {
		t.Errorf("error = %v, want mention of decode", err)
	}
}

// TestCheckConnection_InvalidBaseURL table-drives base_url validation,
// mirroring TestCustomProviderCreate_BaseURLValidation.
func TestCheckConnection_InvalidBaseURL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		baseURL string
	}{
		{"empty", ""},
		{"ftp scheme", "ftp://x"},
		{"garbage", "notaurl"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			svc := setupCustomProviderService(t)
			_, err := svc.CheckConnection(tc.baseURL, "", "")
			if err == nil {
				t.Fatalf("CheckConnection(base_url=%q) succeeded, want error", tc.baseURL)
			}
			if !strings.Contains(err.Error(), "invalid base_url") && !strings.Contains(err.Error(), "base_url is required") {
				t.Errorf("error = %v, want base_url validation message", err)
			}
		})
	}
}

// TestCheckConnection_APIKeyAuthorizationHeader verifies an empty api_key
// omits the Authorization header entirely, while a non-empty one sends a
// Bearer token.
func TestCheckConnection_APIKeyAuthorizationHeader(t *testing.T) {
	t.Parallel()

	t.Run("empty api_key omits header", func(t *testing.T) {
		t.Parallel()
		var sawAuth bool
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "" {
				sawAuth = true
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]string{}})
		}))
		t.Cleanup(srv.Close)

		svc := setupCustomProviderService(t)
		if _, err := svc.CheckConnection(srv.URL, "", ""); err != nil {
			t.Fatalf("CheckConnection: %v", err)
		}
		if sawAuth {
			t.Error("Authorization header present, want none for empty api_key")
		}
	})

	t.Run("non-empty api_key sends Bearer header", func(t *testing.T) {
		t.Parallel()
		var gotAuth string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotAuth = r.Header.Get("Authorization")
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]string{}})
		}))
		t.Cleanup(srv.Close)

		svc := setupCustomProviderService(t)
		if _, err := svc.CheckConnection(srv.URL, "sk-test-key", ""); err != nil {
			t.Fatalf("CheckConnection: %v", err)
		}
		if gotAuth != "Bearer sk-test-key" {
			t.Errorf("Authorization = %q, want %q", gotAuth, "Bearer sk-test-key")
		}
	})

	t.Run("whitespace-only api_key omits header", func(t *testing.T) {
		t.Parallel()
		var sawAuth bool
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "" {
				sawAuth = true
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]string{}})
		}))
		t.Cleanup(srv.Close)

		svc := setupCustomProviderService(t)
		if _, err := svc.CheckConnection(srv.URL, "   ", ""); err != nil {
			t.Fatalf("CheckConnection: %v", err)
		}
		if sawAuth {
			t.Error("Authorization header present, want none for whitespace-only api_key")
		}
	})
}
