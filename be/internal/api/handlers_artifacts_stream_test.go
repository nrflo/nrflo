package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ── handleDownloadArtifact ────────────────────────────────────────────────────

func TestHandleDownloadArtifact_MissingAID(t *testing.T) {
	s, _ := newArtifactHandlerServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/artifacts//download", nil)
	rr := httptest.NewRecorder()
	s.handleDownloadArtifact(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestHandleDownloadArtifact_NotFound(t *testing.T) {
	s, _ := newArtifactHandlerServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/artifacts/no-such-id/download", nil)
	req.SetPathValue("aid", "no-such-id")
	rr := httptest.NewRecorder()
	s.handleDownloadArtifact(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

func TestHandleDownloadArtifact_HappyPath(t *testing.T) {
	s, dir := newArtifactHandlerServer(t)
	aid := seedArtifactForHandler(t, s.pool, dir, "proj-dl", "wfi-dl", "report.txt", "report content")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/artifacts/"+aid+"/download", nil)
	req.SetPathValue("aid", aid)
	rr := httptest.NewRecorder()
	s.handleDownloadArtifact(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}

	ct := rr.Header().Get("Content-Type")
	if ct == "" {
		t.Error("Content-Type header should be set")
	}

	cd := rr.Header().Get("Content-Disposition")
	if !strings.Contains(cd, "attachment") {
		t.Errorf("Content-Disposition = %q, want 'attachment'", cd)
	}
	if !strings.Contains(cd, "report.txt") {
		t.Errorf("Content-Disposition = %q, want filename=report.txt", cd)
	}

	body, err := io.ReadAll(rr.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if string(body) != "report content" {
		t.Errorf("body = %q, want %q", body, "report content")
	}
}

// ── handleDeleteArtifact ──────────────────────────────────────────────────────

func TestHandleDeleteArtifact_MissingAID(t *testing.T) {
	s, _ := newArtifactHandlerServer(t)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/artifacts/", nil)
	rr := httptest.NewRecorder()
	s.handleDeleteArtifact(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestHandleDeleteArtifact_NotFound(t *testing.T) {
	s, _ := newArtifactHandlerServer(t)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/artifacts/no-such-id", nil)
	req.SetPathValue("aid", "no-such-id")
	rr := httptest.NewRecorder()
	s.handleDeleteArtifact(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

func TestHandleDeleteArtifact_HappyPath(t *testing.T) {
	s, dir := newArtifactHandlerServer(t)
	aid := seedArtifactForHandler(t, s.pool, dir, "proj-del2", "wfi-del2", "delete-me.txt", "data")

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/artifacts/"+aid, nil)
	req.SetPathValue("aid", aid)
	rr := httptest.NewRecorder()
	s.handleDeleteArtifact(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}

	// Second delete should return 404
	req2 := httptest.NewRequest(http.MethodDelete, "/api/v1/artifacts/"+aid, nil)
	req2.SetPathValue("aid", aid)
	rr2 := httptest.NewRecorder()
	s.handleDeleteArtifact(rr2, req2)
	if rr2.Code != http.StatusNotFound {
		t.Errorf("second delete status = %d, want 404", rr2.Code)
	}
}
