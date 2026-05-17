package api

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/service"
	"be/internal/types"
	"be/internal/ws"
)

// newArtifactHandlerServer creates a Server with pool, dataPath, and wsHub for artifact tests.
func newArtifactHandlerServer(t *testing.T) (*Server, string) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	if err := apiCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	dataPath := filepath.Join(dir, "nrflo.data")
	t.Setenv("NRFLO_HOME", dir)
	hub := ws.NewHub(clock.Real())
	go hub.Run()
	t.Cleanup(hub.Stop)
	return &Server{pool: pool, clock: clock.Real(), wsHub: hub, dataPath: dataPath}, dir
}

// buildMultipartUpload creates a multipart/form-data POST request with a "file" field.
func buildMultipartUpload(t *testing.T, projectID, fileName, content string) *http.Request {
	t.Helper()
	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	fw, err := w.CreateFormFile("file", fileName)
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := fw.Write([]byte(content)); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	w.Close()
	req := httptest.NewRequest(http.MethodPost,
		withProject("/api/v1/artifact-uploads", projectID), body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	return req
}

// seedArtifactForHandler creates project+wfi+artifact in the DB and storage.
// Returns the artifact ID.
func seedArtifactForHandler(t *testing.T, pool *db.Pool, dir, projID, wfiID, name, content string) string {
	t.Helper()
	now := "2025-01-01T00:00:00Z"
	for _, q := range []struct {
		sql  string
		args []interface{}
	}{
		{`INSERT INTO projects (id, name, created_at, updated_at) VALUES (?, ?, ?, ?)`, []interface{}{projID, projID, now, now}},
		{`INSERT INTO workflows (project_id, id, description, scope_type, created_at, updated_at) VALUES (?, 'wf-' || ?, '', 'project', ?, ?)`, []interface{}{projID, projID, now, now}},
		{`INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, scope_type, findings, created_at, updated_at) VALUES (?, ?, '', 'wf-' || ?, 'active', 'project', '{}', ?, ?)`, []interface{}{wfiID, projID, projID, now, now}},
	} {
		if _, err := pool.Exec(q.sql, q.args...); err != nil {
			t.Fatalf("seedArtifactForHandler: %v", err)
		}
	}
	dataPath := filepath.Join(dir, "nrflo.data")
	svc := service.NewArtifactService(pool, clock.Real(), nil, dataPath)
	ctx := context.Background()
	data := []byte(content)
	uid, err := svc.StageUpload(ctx, projID, "test-user", name, bytes.NewReader(data), int64(len(data)), "text/plain")
	if err != nil {
		t.Fatalf("StageUpload: %v", err)
	}
	if err := svc.AttachInputArtifacts(ctx, projID, wfiID, []types.InputArtifactRef{{UploadID: uid}}); err != nil {
		t.Fatalf("AttachInputArtifacts: %v", err)
	}
	artifacts, err := svc.List(ctx, wfiID)
	if err != nil || len(artifacts) == 0 {
		t.Fatalf("List: %v / len=%d", err, len(artifacts))
	}
	return artifacts[0].ID
}

// ── handleStageUpload ─────────────────────────────────────────────────────────

func TestHandleStageUpload_MissingProject(t *testing.T) {
	s, _ := newArtifactHandlerServer(t)
	req := buildMultipartUpload(t, "", "test.txt", "content")
	// Clear the project param
	req.URL.RawQuery = ""
	rr := httptest.NewRecorder()
	s.handleStageUpload(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "X-Project")
}

func TestHandleStageUpload_MissingFile(t *testing.T) {
	s, _ := newArtifactHandlerServer(t)
	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	w.Close()
	req := httptest.NewRequest(http.MethodPost,
		withProject("/api/v1/artifact-uploads", "proj1"), body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	rr := httptest.NewRecorder()
	s.handleStageUpload(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "file field required")
}

func TestHandleStageUpload_HappyPath(t *testing.T) {
	s, _ := newArtifactHandlerServer(t)
	req := buildMultipartUpload(t, "proj-upload", "hello.txt", "file content here")
	rr := httptest.NewRecorder()
	s.handleStageUpload(rr, req)
	if rr.Code != http.StatusCreated {
		t.Errorf("status = %d, want 201; body: %s", rr.Code, rr.Body.String())
	}
	var resp types.ArtifactUploadResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.UploadID == "" {
		t.Error("upload_id should not be empty")
	}
	if resp.Name != "hello.txt" {
		t.Errorf("name = %q, want hello.txt", resp.Name)
	}
	if resp.SizeBytes == 0 {
		t.Error("size_bytes should be > 0")
	}
}

// ── handleCancelUpload ────────────────────────────────────────────────────────

func TestHandleCancelUpload_MissingUploadID(t *testing.T) {
	s, _ := newArtifactHandlerServer(t)
	req := httptest.NewRequest(http.MethodDelete,
		withProject("/api/v1/artifact-uploads/", "proj1"), nil)
	rr := httptest.NewRecorder()
	s.handleCancelUpload(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestHandleCancelUpload_NotFound(t *testing.T) {
	s, _ := newArtifactHandlerServer(t)
	req := httptest.NewRequest(http.MethodDelete,
		withProject("/api/v1/artifact-uploads/no-such-id", "proj1"), nil)
	req.SetPathValue("upload_id", "no-such-id")
	rr := httptest.NewRecorder()
	s.handleCancelUpload(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

func TestHandleCancelUpload_WrongProject(t *testing.T) {
	s, _ := newArtifactHandlerServer(t)
	// Stage an upload under "proj-a"
	uploadReq := buildMultipartUpload(t, "proj-a", "file.txt", "data")
	uploadRR := httptest.NewRecorder()
	s.handleStageUpload(uploadRR, uploadReq)
	if uploadRR.Code != http.StatusCreated {
		t.Fatalf("stage upload: %d %s", uploadRR.Code, uploadRR.Body.String())
	}
	var resp types.ArtifactUploadResponse
	json.NewDecoder(uploadRR.Body).Decode(&resp)

	// Cancel from a different project
	req := httptest.NewRequest(http.MethodDelete,
		withProject("/api/v1/artifact-uploads/"+resp.UploadID, "proj-b"), nil)
	req.SetPathValue("upload_id", resp.UploadID)
	rr := httptest.NewRecorder()
	s.handleCancelUpload(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rr.Code)
	}
}

func TestHandleCancelUpload_HappyPath(t *testing.T) {
	s, _ := newArtifactHandlerServer(t)
	uploadReq := buildMultipartUpload(t, "proj-cancel", "del.txt", "bye")
	uploadRR := httptest.NewRecorder()
	s.handleStageUpload(uploadRR, uploadReq)
	if uploadRR.Code != http.StatusCreated {
		t.Fatalf("stage upload: %d", uploadRR.Code)
	}
	var resp types.ArtifactUploadResponse
	json.NewDecoder(uploadRR.Body).Decode(&resp)

	req := httptest.NewRequest(http.MethodDelete,
		withProject("/api/v1/artifact-uploads/"+resp.UploadID, "proj-cancel"), nil)
	req.SetPathValue("upload_id", resp.UploadID)
	rr := httptest.NewRecorder()
	s.handleCancelUpload(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
}

// ── handleListArtifacts ───────────────────────────────────────────────────────

func TestHandleListArtifacts_MissingIID(t *testing.T) {
	s, _ := newArtifactHandlerServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/workflow-instances//artifacts", nil)
	rr := httptest.NewRecorder()
	s.handleListArtifacts(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestHandleListArtifacts_Empty(t *testing.T) {
	s, _ := newArtifactHandlerServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/workflow-instances/no-wfi/artifacts", nil)
	req.SetPathValue("iid", "no-wfi")
	rr := httptest.NewRecorder()
	s.handleListArtifacts(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	var dtos []types.ArtifactDTO
	if err := json.NewDecoder(rr.Body).Decode(&dtos); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(dtos) != 0 {
		t.Errorf("dtos len = %d, want 0", len(dtos))
	}
}

func TestHandleListArtifacts_HappyPath(t *testing.T) {
	s, dir := newArtifactHandlerServer(t)
	seedArtifactForHandler(t, s.pool, dir, "proj-list", "wfi-list", "result.txt", "results data")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/workflow-instances/wfi-list/artifacts", nil)
	req.SetPathValue("iid", "wfi-list")
	rr := httptest.NewRecorder()
	s.handleListArtifacts(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	var dtos []types.ArtifactDTO
	if err := json.NewDecoder(rr.Body).Decode(&dtos); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(dtos) != 1 {
		t.Fatalf("dtos len = %d, want 1", len(dtos))
	}
	if dtos[0].Name != "result.txt" {
		t.Errorf("name = %q, want result.txt", dtos[0].Name)
	}
	if dtos[0].Source != "input" {
		t.Errorf("source = %q, want input", dtos[0].Source)
	}
	if dtos[0].CreatedAt == "" {
		t.Error("created_at should not be empty")
	}
}
