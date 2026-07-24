package consoleui

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestClient_SetYolo_MethodBySessionState verifies SetYolo(true) issues a
// POST and SetYolo(false) issues a DELETE against the chat's own yolo route.
func TestClient_SetYolo_MethodBySessionState(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)

	c := NewClient(Config{BaseURL: srv.URL, Session: "sess-1"})

	if err := c.SetYolo(context.Background(), true); err != nil {
		t.Fatalf("SetYolo(true): %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("SetYolo(true) method = %q, want POST", gotMethod)
	}
	if gotPath != "/api/v1/console/chats/sess-1/yolo" {
		t.Errorf("SetYolo(true) path = %q, want /api/v1/console/chats/sess-1/yolo", gotPath)
	}

	if err := c.SetYolo(context.Background(), false); err != nil {
		t.Fatalf("SetYolo(false): %v", err)
	}
	if gotMethod != http.MethodDelete {
		t.Errorf("SetYolo(false) method = %q, want DELETE", gotMethod)
	}
}

// TestClient_SetYolo_PropagatesServerError verifies a non-2xx response
// surfaces as an error rather than being swallowed.
func TestClient_SetYolo_PropagatesServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	c := NewClient(Config{BaseURL: srv.URL, Session: "sess-missing"})
	if err := c.SetYolo(context.Background(), true); err == nil {
		t.Error("SetYolo() with a 404 response = nil error, want non-nil")
	}
}
