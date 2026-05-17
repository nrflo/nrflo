package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"be/internal/artifact"
)

func TestHandleGetProjectArtifactStorage_DefaultInternal(t *testing.T) {
	t.Parallel()
	s, projectID := newProjectSettingsServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+projectID+"/settings/artifact-storage", nil)
	req.SetPathValue("id", projectID)
	rr := httptest.NewRecorder()
	s.handleGetProjectArtifactStorage(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	cfg := decodeArtifactStorageResponse(t, rr)
	if cfg.Mode != artifact.ModeInternal {
		t.Errorf("default Mode = %q, want %q", cfg.Mode, artifact.ModeInternal)
	}
}

func TestHandleGetProjectArtifactStorage_MissingProjectID(t *testing.T) {
	t.Parallel()
	s, _ := newProjectSettingsServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects//settings/artifact-storage", nil)
	rr := httptest.NewRecorder()
	s.handleGetProjectArtifactStorage(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestHandlePutProjectArtifactStorage_S3Rejected(t *testing.T) {
	t.Parallel()
	s, projectID := newProjectSettingsServer(t)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/projects/"+projectID+"/settings/artifact-storage",
		strings.NewReader(`{"mode":"s3"}`))
	req.SetPathValue("id", projectID)
	rr := httptest.NewRecorder()
	s.handlePutProjectArtifactStorage(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for s3 mode; body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "not yet implemented") {
		t.Errorf("error body should mention 'not yet implemented'; got: %s", rr.Body.String())
	}
}

func TestHandlePutProjectArtifactStorage_R2RoundTrip(t *testing.T) {
	t.Parallel()
	s, projectID := newProjectSettingsServer(t)

	body := `{"mode":"cloudflare_r2","account_id":"acct","bucket":"bkt","access_key_ref":"env:AK","secret_key_ref":"env:SK"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/projects/"+projectID+"/settings/artifact-storage",
		strings.NewReader(body))
	req.SetPathValue("id", projectID)
	rr := httptest.NewRecorder()
	s.handlePutProjectArtifactStorage(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	cfg := decodeArtifactStorageResponse(t, rr)
	if cfg.Mode != artifact.ModeR2 {
		t.Errorf("Mode = %q, want cloudflare_r2", cfg.Mode)
	}
	if cfg.Bucket != "bkt" {
		t.Errorf("Bucket = %q, want bkt", cfg.Bucket)
	}
}

func TestHandlePutProjectArtifactStorage_LiteralSecretRedactedOnResponse(t *testing.T) {
	t.Parallel()
	s, projectID := newProjectSettingsServer(t)

	body := `{"mode":"cloudflare_r2","account_id":"acct","bucket":"bkt","access_key_ref":"literal:mykey","secret_key_ref":"literal:mysecret"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/projects/"+projectID+"/settings/artifact-storage",
		strings.NewReader(body))
	req.SetPathValue("id", projectID)
	rr := httptest.NewRecorder()
	s.handlePutProjectArtifactStorage(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	bodyText := rr.Body.String()
	if strings.Contains(bodyText, "mykey") || strings.Contains(bodyText, "mysecret") {
		t.Errorf("PUT response must not echo plaintext literal values; got: %s", bodyText)
	}
	if !strings.Contains(bodyText, "literal:***") {
		t.Errorf("PUT response must redact as literal:***; got: %s", bodyText)
	}
}

func TestHandlePutProjectArtifactStorage_GetAfterPutRedacts(t *testing.T) {
	t.Parallel()
	s, projectID := newProjectSettingsServer(t)

	putBody := `{"mode":"cloudflare_r2","account_id":"acct","bucket":"bkt","access_key_ref":"literal:rawkey","secret_key_ref":"literal:rawsecret"}`
	putReq := httptest.NewRequest(http.MethodPut, "/api/v1/projects/"+projectID+"/settings/artifact-storage",
		strings.NewReader(putBody))
	putReq.SetPathValue("id", projectID)
	putRR := httptest.NewRecorder()
	s.handlePutProjectArtifactStorage(putRR, putReq)
	if putRR.Code != http.StatusOK {
		t.Fatalf("PUT status = %d", putRR.Code)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+projectID+"/settings/artifact-storage", nil)
	getReq.SetPathValue("id", projectID)
	getRR := httptest.NewRecorder()
	s.handleGetProjectArtifactStorage(getRR, getReq)
	if getRR.Code != http.StatusOK {
		t.Fatalf("GET status = %d", getRR.Code)
	}
	bodyText := getRR.Body.String()
	if strings.Contains(bodyText, "rawkey") || strings.Contains(bodyText, "rawsecret") {
		t.Errorf("GET response must not expose literal values after PUT; got: %s", bodyText)
	}
}

func TestHandlePutProjectArtifactStorage_BadBody(t *testing.T) {
	t.Parallel()
	s, projectID := newProjectSettingsServer(t)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/projects/"+projectID+"/settings/artifact-storage",
		strings.NewReader(`{not json`))
	req.SetPathValue("id", projectID)
	rr := httptest.NewRecorder()
	s.handlePutProjectArtifactStorage(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for malformed JSON", rr.Code)
	}
}

func TestHandlePutProjectArtifactStorage_MissingProjectID(t *testing.T) {
	t.Parallel()
	s, _ := newProjectSettingsServer(t)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/projects//settings/artifact-storage",
		strings.NewReader(`{"mode":"internal"}`))
	rr := httptest.NewRecorder()
	s.handlePutProjectArtifactStorage(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestHandlePutProjectArtifactStorage_SwitchToInternal(t *testing.T) {
	t.Parallel()
	s, projectID := newProjectSettingsServer(t)

	putBody := `{"mode":"cloudflare_r2","account_id":"acct","bucket":"bkt","access_key_ref":"env:AK","secret_key_ref":"env:SK"}`
	putReq := httptest.NewRequest(http.MethodPut, "/api/v1/projects/"+projectID+"/settings/artifact-storage",
		strings.NewReader(putBody))
	putReq.SetPathValue("id", projectID)
	putRR := httptest.NewRecorder()
	s.handlePutProjectArtifactStorage(putRR, putReq)
	if putRR.Code != http.StatusOK {
		t.Fatalf("PUT r2 status = %d", putRR.Code)
	}

	switchReq := httptest.NewRequest(http.MethodPut, "/api/v1/projects/"+projectID+"/settings/artifact-storage",
		strings.NewReader(`{"mode":"internal"}`))
	switchReq.SetPathValue("id", projectID)
	switchRR := httptest.NewRecorder()
	s.handlePutProjectArtifactStorage(switchRR, switchReq)
	if switchRR.Code != http.StatusOK {
		t.Fatalf("PUT internal status = %d; body=%s", switchRR.Code, switchRR.Body.String())
	}
	cfg := decodeArtifactStorageResponse(t, switchRR)
	if cfg.Mode != artifact.ModeInternal {
		t.Errorf("Mode after switch = %q, want internal", cfg.Mode)
	}
}
