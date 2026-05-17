package service

import (
	"path/filepath"
	"strings"
	"testing"

	"be/internal/artifact"
	"be/internal/clock"
	"be/internal/db"
)

func setupArtifactStorageTestEnv(t *testing.T) (*GlobalSettingsService, string) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "artifact_storage_test.db")
	if err := svcCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	const projectID = "proj-art-test"
	return NewGlobalSettingsService(pool, clock.Real()), projectID
}

func TestGetArtifactStorage_DefaultIsInternal(t *testing.T) {
	t.Parallel()
	svc, projectID := setupArtifactStorageTestEnv(t)

	cfg, err := svc.GetArtifactStorage(projectID)
	if err != nil {
		t.Fatalf("GetArtifactStorage() error: %v", err)
	}
	if cfg.Mode != artifact.ModeInternal {
		t.Errorf("default Mode = %q, want %q", cfg.Mode, artifact.ModeInternal)
	}
}

func TestArtifactStorage_RoundTripR2(t *testing.T) {
	t.Parallel()
	svc, projectID := setupArtifactStorageTestEnv(t)

	want := artifact.Config{
		Mode:         artifact.ModeR2,
		AccountID:    "acct-abc123",
		Bucket:       "my-r2-bucket",
		Prefix:       "myprefix/",
		AccessKeyRef: "literal:accesskey",
		SecretKeyRef: "literal:secretkey",
	}
	if err := svc.SetArtifactStorage(projectID, want); err != nil {
		t.Fatalf("SetArtifactStorage() error: %v", err)
	}

	got, err := svc.GetArtifactStorage(projectID)
	if err != nil {
		t.Fatalf("GetArtifactStorage() error: %v", err)
	}
	if got.Mode != want.Mode {
		t.Errorf("Mode = %q, want %q", got.Mode, want.Mode)
	}
	if got.AccountID != want.AccountID {
		t.Errorf("AccountID = %q, want %q", got.AccountID, want.AccountID)
	}
	if got.Bucket != want.Bucket {
		t.Errorf("Bucket = %q, want %q", got.Bucket, want.Bucket)
	}
	if got.Prefix != want.Prefix {
		t.Errorf("Prefix = %q, want %q", got.Prefix, want.Prefix)
	}
	if got.AccessKeyRef != want.AccessKeyRef {
		t.Errorf("AccessKeyRef = %q, want %q", got.AccessKeyRef, want.AccessKeyRef)
	}
	if got.SecretKeyRef != want.SecretKeyRef {
		t.Errorf("SecretKeyRef = %q, want %q", got.SecretKeyRef, want.SecretKeyRef)
	}
}

func TestSetArtifactStorage_S3Rejected(t *testing.T) {
	t.Parallel()
	svc, projectID := setupArtifactStorageTestEnv(t)

	err := svc.SetArtifactStorage(projectID, artifact.Config{Mode: artifact.ModeS3})
	if err == nil {
		t.Fatal("SetArtifactStorage(s3) expected error, got nil")
	}
	if err.Error() != "s3 backend not yet implemented" {
		t.Errorf("error = %q, want %q", err.Error(), "s3 backend not yet implemented")
	}

	// Config must remain at default (internal) after rejection.
	cfg, err := svc.GetArtifactStorage(projectID)
	if err != nil {
		t.Fatalf("GetArtifactStorage() after rejected s3: %v", err)
	}
	if cfg.Mode != artifact.ModeInternal {
		t.Errorf("mode after s3 rejection = %q, want %q", cfg.Mode, artifact.ModeInternal)
	}
}

func TestSetArtifactStorage_R2MissingFields(t *testing.T) {
	t.Parallel()
	svc, projectID := setupArtifactStorageTestEnv(t)

	fullR2 := artifact.Config{
		Mode:         artifact.ModeR2,
		AccountID:    "acct",
		Bucket:       "bkt",
		AccessKeyRef: "literal:ak",
		SecretKeyRef: "literal:sk",
	}

	tests := []struct {
		name        string
		modify      func(c *artifact.Config)
		wantMissing string
	}{
		{
			name:        "missing_account_id",
			modify:      func(c *artifact.Config) { c.AccountID = "" },
			wantMissing: "account_id",
		},
		{
			name:        "missing_bucket",
			modify:      func(c *artifact.Config) { c.Bucket = "" },
			wantMissing: "bucket",
		},
		{
			name:        "missing_access_key_ref",
			modify:      func(c *artifact.Config) { c.AccessKeyRef = "" },
			wantMissing: "access_key_ref",
		},
		{
			name:        "missing_secret_key_ref",
			modify:      func(c *artifact.Config) { c.SecretKeyRef = "" },
			wantMissing: "secret_key_ref",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			cfg := fullR2
			tc.modify(&cfg)
			err := svc.SetArtifactStorage(projectID, cfg)
			if err == nil {
				t.Fatalf("SetArtifactStorage() expected error for %s, got nil", tc.wantMissing)
			}
			if !strings.Contains(err.Error(), tc.wantMissing) {
				t.Errorf("error = %q, want it to contain %q", err.Error(), tc.wantMissing)
			}
		})
	}
}

func TestGetArtifactStorageRedacted_MasksLiteral(t *testing.T) {
	t.Parallel()
	svc, projectID := setupArtifactStorageTestEnv(t)

	if err := svc.SetArtifactStorage(projectID, artifact.Config{
		Mode:         artifact.ModeR2,
		AccountID:    "acct",
		Bucket:       "bkt",
		AccessKeyRef: "literal:myrealkey",
		SecretKeyRef: "literal:mysecretvalue",
	}); err != nil {
		t.Fatalf("SetArtifactStorage(): %v", err)
	}

	redacted, err := svc.GetArtifactStorageRedacted(projectID)
	if err != nil {
		t.Fatalf("GetArtifactStorageRedacted(): %v", err)
	}
	if redacted.AccessKeyRef != "literal:***" {
		t.Errorf("AccessKeyRef = %q, want %q", redacted.AccessKeyRef, "literal:***")
	}
	if redacted.SecretKeyRef != "literal:***" {
		t.Errorf("SecretKeyRef = %q, want %q", redacted.SecretKeyRef, "literal:***")
	}
}

func TestGetArtifactStorageRedacted_PreservesEnvAndFile(t *testing.T) {
	t.Parallel()
	svc, projectID := setupArtifactStorageTestEnv(t)

	if err := svc.SetArtifactStorage(projectID, artifact.Config{
		Mode:         artifact.ModeR2,
		AccountID:    "acct",
		Bucket:       "bkt",
		AccessKeyRef: "env:MY_ACCESS_KEY",
		SecretKeyRef: "file:/etc/secrets/sk",
	}); err != nil {
		t.Fatalf("SetArtifactStorage(): %v", err)
	}

	redacted, err := svc.GetArtifactStorageRedacted(projectID)
	if err != nil {
		t.Fatalf("GetArtifactStorageRedacted(): %v", err)
	}
	if redacted.AccessKeyRef != "env:MY_ACCESS_KEY" {
		t.Errorf("AccessKeyRef = %q, want %q", redacted.AccessKeyRef, "env:MY_ACCESS_KEY")
	}
	if redacted.SecretKeyRef != "file:/etc/secrets/sk" {
		t.Errorf("SecretKeyRef = %q, want %q", redacted.SecretKeyRef, "file:/etc/secrets/sk")
	}
}

func TestGetArtifactStorageRedacted_DefaultNoRefs(t *testing.T) {
	t.Parallel()
	svc, projectID := setupArtifactStorageTestEnv(t)

	// Default config has no refs — redacted call should succeed and return internal mode.
	cfg, err := svc.GetArtifactStorageRedacted(projectID)
	if err != nil {
		t.Fatalf("GetArtifactStorageRedacted() default: %v", err)
	}
	if cfg.Mode != artifact.ModeInternal {
		t.Errorf("default redacted Mode = %q, want %q", cfg.Mode, artifact.ModeInternal)
	}
	if cfg.AccessKeyRef != "" || cfg.SecretKeyRef != "" {
		t.Errorf("unexpected refs in default config: ak=%q sk=%q", cfg.AccessKeyRef, cfg.SecretKeyRef)
	}
}

func TestSetArtifactStorage_UnknownModeRejected(t *testing.T) {
	t.Parallel()
	svc, projectID := setupArtifactStorageTestEnv(t)

	err := svc.SetArtifactStorage(projectID, artifact.Config{Mode: artifact.StorageMode("gcs")})
	if err == nil {
		t.Fatal("SetArtifactStorage(unknown) expected error, got nil")
	}
}

func TestArtifactStorage_SwitchBackToInternal(t *testing.T) {
	t.Parallel()
	svc, projectID := setupArtifactStorageTestEnv(t)

	// Configure R2.
	if err := svc.SetArtifactStorage(projectID, artifact.Config{
		Mode:         artifact.ModeR2,
		AccountID:    "acct",
		Bucket:       "bkt",
		AccessKeyRef: "literal:ak",
		SecretKeyRef: "literal:sk",
	}); err != nil {
		t.Fatalf("SetArtifactStorage(r2): %v", err)
	}

	// Switch back to internal.
	if err := svc.SetArtifactStorage(projectID, artifact.Config{Mode: artifact.ModeInternal}); err != nil {
		t.Fatalf("SetArtifactStorage(internal): %v", err)
	}

	cfg, err := svc.GetArtifactStorage(projectID)
	if err != nil {
		t.Fatalf("GetArtifactStorage(): %v", err)
	}
	if cfg.Mode != artifact.ModeInternal {
		t.Errorf("Mode after switch = %q, want %q", cfg.Mode, artifact.ModeInternal)
	}
}
