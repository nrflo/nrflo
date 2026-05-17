package service

import (
	"path/filepath"
	"testing"

	"be/internal/artifact"
	"be/internal/clock"
	"be/internal/db"
)

func setupArtifactTestEnv(t *testing.T) (*GlobalSettingsService, string) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "artifact_test.db")
	if err := svcCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return NewGlobalSettingsService(pool, clock.Real()), "proj-artifact-test"
}

func TestSetArtifactStorage_SentinelPreservesLiteralKey(t *testing.T) {
	t.Parallel()
	svc, projectID := setupArtifactTestEnv(t)

	initial := artifact.Config{
		Mode:         artifact.ModeR2,
		AccountID:    "acct",
		Bucket:       "bkt",
		AccessKeyRef: "literal:rawkey",
		SecretKeyRef: "literal:rawsecret",
	}
	if err := svc.SetArtifactStorage(projectID, initial); err != nil {
		t.Fatalf("initial SetArtifactStorage: %v", err)
	}

	// Simulate PUT from UI where user didn't change secrets — sends the redacted sentinel
	update := artifact.Config{
		Mode:         artifact.ModeR2,
		AccountID:    "acct",
		Bucket:       "newbucket",
		AccessKeyRef: artifact.RedactedSentinel,
		SecretKeyRef: artifact.RedactedSentinel,
	}
	if err := svc.SetArtifactStorage(projectID, update); err != nil {
		t.Fatalf("sentinel SetArtifactStorage: %v", err)
	}

	got, err := svc.GetArtifactStorage(projectID)
	if err != nil {
		t.Fatalf("GetArtifactStorage: %v", err)
	}
	if got.Bucket != "newbucket" {
		t.Errorf("Bucket = %q, want newbucket", got.Bucket)
	}
	if got.AccessKeyRef != "literal:rawkey" {
		t.Errorf("AccessKeyRef = %q, want literal:rawkey (sentinel must not be stored)", got.AccessKeyRef)
	}
	if got.SecretKeyRef != "literal:rawsecret" {
		t.Errorf("SecretKeyRef = %q, want literal:rawsecret (sentinel must not be stored)", got.SecretKeyRef)
	}
}

func TestSetArtifactStorage_SentinelWithNoExistingKeyFails(t *testing.T) {
	t.Parallel()
	svc, projectID := setupArtifactTestEnv(t)

	// Send sentinel with no existing stored key — should fail validation (empty after substitution)
	cfg := artifact.Config{
		Mode:         artifact.ModeR2,
		AccountID:    "acct",
		Bucket:       "bkt",
		AccessKeyRef: artifact.RedactedSentinel,
		SecretKeyRef: artifact.RedactedSentinel,
	}
	err := svc.SetArtifactStorage(projectID, cfg)
	if err == nil {
		t.Fatal("expected error for sentinel with no existing key, got nil")
	}
}
