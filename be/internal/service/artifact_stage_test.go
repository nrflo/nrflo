package service

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/types"
)

// newArtifactSvcEnv creates an isolated ArtifactService with a temp DB and NRFLO_HOME.
func newArtifactSvcEnv(t *testing.T) (*ArtifactService, *fakeHub, string) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	if err := svcCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	dataPath := filepath.Join(dir, "nrflo.data")
	t.Setenv("NRFLO_HOME", dir)
	hub := &fakeHub{}
	svc := NewArtifactService(pool, clock.Real(), hub, dataPath)
	return svc, hub, dir
}

// seedProjWFI inserts a project + workflow + workflow_instance into the test DB.
func seedProjWFI(t *testing.T, pool *db.Pool, projID, wfiID string) {
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
			t.Fatalf("seedProjWFI(%s,%s): %v", projID, wfiID, err)
		}
	}
}

// stageOne stages one upload and returns the uploadID.
func stageOne(t *testing.T, svc *ArtifactService, projID, name, content string) string {
	t.Helper()
	data := []byte(content)
	uid, err := svc.StageUpload(context.Background(), projID, "test-user", name, bytes.NewReader(data), int64(len(data)), "text/plain")
	if err != nil {
		t.Fatalf("StageUpload(%s): %v", name, err)
	}
	return uid
}

func TestArtifact_StageUpload_WritesFileAndSidecar(t *testing.T) {
	svc, _, dir := newArtifactSvcEnv(t)
	content := []byte("hello artifact")

	uploadID, err := svc.StageUpload(context.Background(), "proj1", "user1", "hello.txt",
		bytes.NewReader(content), int64(len(content)), "text/plain")
	if err != nil {
		t.Fatalf("StageUpload: %v", err)
	}
	if uploadID == "" {
		t.Error("uploadID empty")
	}

	filePath := filepath.Join(dir, "tmp", "uploads", uploadID, "file")
	if _, err := os.Stat(filePath); err != nil {
		t.Errorf("staged file missing: %v", err)
	}

	meta, err := svc.ReadUploadMeta(uploadID)
	if err != nil {
		t.Fatalf("ReadUploadMeta: %v", err)
	}
	if meta.ProjectID != "proj1" {
		t.Errorf("meta.ProjectID = %q, want proj1", meta.ProjectID)
	}
	if meta.Name != "hello.txt" {
		t.Errorf("meta.Name = %q, want hello.txt", meta.Name)
	}
	if meta.ContentType != "text/plain" {
		t.Errorf("meta.ContentType = %q, want text/plain", meta.ContentType)
	}
	if meta.Size != int64(len(content)) {
		t.Errorf("meta.Size = %d, want %d", meta.Size, len(content))
	}
	if meta.UserID != "user1" {
		t.Errorf("meta.UserID = %q, want user1", meta.UserID)
	}
}

func TestArtifact_ReadUploadMeta_NotFound(t *testing.T) {
	svc, _, _ := newArtifactSvcEnv(t)
	_, err := svc.ReadUploadMeta("no-such-id")
	if err == nil {
		t.Fatal("expected error for missing upload")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want 'not found'", err.Error())
	}
}

func TestArtifact_CancelUpload_RemovesDir(t *testing.T) {
	svc, _, dir := newArtifactSvcEnv(t)
	uid := stageOne(t, svc, "proj1", "cancel.txt", "content")
	uploadDir := filepath.Join(dir, "tmp", "uploads", uid)

	if _, err := os.Stat(uploadDir); err != nil {
		t.Fatalf("upload dir should exist before cancel: %v", err)
	}
	if err := svc.CancelUpload(context.Background(), uid); err != nil {
		t.Fatalf("CancelUpload: %v", err)
	}
	if _, err := os.Stat(uploadDir); !os.IsNotExist(err) {
		t.Error("upload dir should be removed after cancel")
	}
}

func TestArtifact_AttachInputArtifacts_HappyPath(t *testing.T) {
	svc, _, dir := newArtifactSvcEnv(t)
	ctx := context.Background()
	seedProjWFI(t, svc.pool, "proj-attach", "wfi-attach")

	content := []byte("artifact bytes")
	uid := stageOne(t, svc, "proj-attach", "data.bin", string(content))

	uploads := []types.InputArtifactRef{{UploadID: uid}}
	if err := svc.AttachInputArtifacts(ctx, "proj-attach", "wfi-attach", uploads); err != nil {
		t.Fatalf("AttachInputArtifacts: %v", err)
	}

	// DB row created with source=input
	artifacts, err := svc.List(ctx, "wfi-attach")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(artifacts) != 1 {
		t.Fatalf("artifacts len = %d, want 1", len(artifacts))
	}
	a := artifacts[0]
	if a.Source != model.ArtifactSourceInput {
		t.Errorf("source = %q, want %q", a.Source, model.ArtifactSourceInput)
	}
	if a.Name != "data.bin" {
		t.Errorf("name = %q, want data.bin", a.Name)
	}
	if a.WorkflowInstanceID != "wfi-attach" {
		t.Errorf("wfi_id = %q, want wfi-attach", a.WorkflowInstanceID)
	}

	// File landed in storage
	storagePath := filepath.Join(dir, "projects", "proj-attach", "artifacts", a.PathKey)
	if _, err := os.Stat(storagePath); err != nil {
		t.Errorf("artifact not in storage: %v", err)
	}

	// Temp dir removed
	uploadDir := filepath.Join(dir, "tmp", "uploads", uid)
	if _, err := os.Stat(uploadDir); !os.IsNotExist(err) {
		t.Error("upload temp dir should be removed after attach")
	}
}

func TestArtifact_AttachInputArtifacts_Rollback(t *testing.T) {
	svc, _, _ := newArtifactSvcEnv(t)
	ctx := context.Background()
	seedProjWFI(t, svc.pool, "proj-rb", "wfi-rb")

	uid1 := stageOne(t, svc, "proj-rb", "good.txt", "good content")

	uploads := []types.InputArtifactRef{
		{UploadID: uid1},
		{UploadID: "missing-upload-id"},
	}
	err := svc.AttachInputArtifacts(ctx, "proj-rb", "wfi-rb", uploads)
	if err == nil {
		t.Fatal("expected error for missing upload")
	}

	// No DB rows left
	artifacts, listErr := svc.List(ctx, "wfi-rb")
	if listErr != nil {
		t.Fatalf("List: %v", listErr)
	}
	if len(artifacts) != 0 {
		t.Errorf("artifacts len = %d after rollback, want 0", len(artifacts))
	}
}

func TestArtifact_AttachInputArtifacts_Empty(t *testing.T) {
	svc, _, _ := newArtifactSvcEnv(t)
	err := svc.AttachInputArtifacts(context.Background(), "proj", "wfi", nil)
	if err != nil {
		t.Errorf("AttachInputArtifacts(nil) = %v, want nil", err)
	}
}

func TestArtifact_AddFromAgent_HappyPath(t *testing.T) {
	svc, hub, dir := newArtifactSvcEnv(t)
	ctx := context.Background()
	seedProjWFI(t, svc.pool, "proj-agent", "wfi-agent")

	data := []byte("agent produced data")
	a, err := svc.AddFromAgent(ctx, "sess-1", "proj-agent", "wfi-agent", "output.txt", "text/plain", data)
	if err != nil {
		t.Fatalf("AddFromAgent: %v", err)
	}
	if a.Source != model.ArtifactSourceAgent {
		t.Errorf("source = %q, want %q", a.Source, model.ArtifactSourceAgent)
	}
	if a.CreatedBySession != "sess-1" {
		t.Errorf("created_by_session = %q, want sess-1", a.CreatedBySession)
	}
	if a.SizeBytes != int64(len(data)) {
		t.Errorf("size_bytes = %d, want %d", a.SizeBytes, len(data))
	}
	if a.Name != "output.txt" {
		t.Errorf("name = %q, want output.txt", a.Name)
	}

	// File in storage
	storagePath := dir + "/projects/proj-agent/artifacts/" + a.PathKey
	if _, statErr := os.Stat(storagePath); statErr != nil {
		t.Errorf("artifact not in storage: %v", statErr)
	}

	// Broadcast emitted
	if len(hub.events) == 0 {
		t.Error("expected broadcast event, got none")
	}
}

func TestArtifact_AddFromAgent_OverSizeReturnsError(t *testing.T) {
	svc, _, _ := newArtifactSvcEnv(t)
	// 32 MiB + 1 byte
	bigData := make([]byte, 32*1024*1024+1)
	_, err := svc.AddFromAgent(context.Background(), "s", "proj", "wfi", "big.bin", "", bigData)
	if err == nil {
		t.Fatal("expected error for oversized artifact")
	}
}
