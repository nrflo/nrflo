package service

import (
	"path/filepath"
	"strings"
	"testing"

	"be/internal/clock"
	"be/internal/db"
)

func setupEnvVarSvc(t *testing.T) (*ProjectEnvVarService, string) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "ev_svc_test.db")
	if err := svcCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	projectID := "proj-ev-svc"
	if _, err := pool.Exec(`INSERT INTO projects (id, name, created_at, updated_at)
		VALUES (?, 'TestProject', datetime('now'), datetime('now'))`, projectID); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	return NewProjectEnvVarService(pool, clock.Real()), projectID
}

func TestProjectEnvVarService_ListReturnsEmptySlice(t *testing.T) {
	svc, projectID := setupEnvVarSvc(t)

	list, err := svc.List(projectID)
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if list == nil {
		t.Error("List() = nil, want empty slice")
	}
	if len(list) != 0 {
		t.Errorf("List() = %d items, want 0", len(list))
	}
}

func TestProjectEnvVarService_UpsertHappyPath(t *testing.T) {
	svc, projectID := setupEnvVarSvc(t)

	v, err := svc.Upsert(projectID, "MY_VAR", "myvalue")
	if err != nil {
		t.Fatalf("Upsert() error: %v", err)
	}
	if v.Name != "MY_VAR" {
		t.Errorf("Name = %q, want MY_VAR", v.Name)
	}
	if v.Value != "myvalue" {
		t.Errorf("Value = %q, want myvalue", v.Value)
	}

	list, err := svc.List(projectID)
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("List() = %d items, want 1", len(list))
	}
}

func TestProjectEnvVarService_UpsertOverwrite(t *testing.T) {
	svc, projectID := setupEnvVarSvc(t)

	if _, err := svc.Upsert(projectID, "OVER_VAR", "original"); err != nil {
		t.Fatalf("Upsert first: %v", err)
	}
	if _, err := svc.Upsert(projectID, "OVER_VAR", "updated"); err != nil {
		t.Fatalf("Upsert second: %v", err)
	}

	list, err := svc.List(projectID)
	if err != nil {
		t.Fatalf("List(): %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("List() = %d items, want 1 after overwrite", len(list))
	}
	if list[0].Value != "updated" {
		t.Errorf("Value = %q, want updated", list[0].Value)
	}
}

func TestProjectEnvVarService_NameValidation(t *testing.T) {
	svc, projectID := setupEnvVarSvc(t)

	cases := []struct {
		name  string
		valid bool
	}{
		{"VALID_NAME", true},
		{"valid_lower", true},
		{"_STARTS_UNDERSCORE", true},
		{"A", true},
		{"A1", true},
		{"MY_VAR_123", true},
		{"", false},
		{"1STARTS_DIGIT", false},
		{"MY-VAR", false},
		{"MY VAR", false},
		{"MY.VAR", false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name+"_valid="+boolStr(tc.valid), func(t *testing.T) {
			_, err := svc.Upsert(projectID, tc.name, "value")
			if tc.valid && err != nil {
				t.Errorf("Upsert(%q) unexpected error: %v", tc.name, err)
			}
			if !tc.valid && err == nil {
				t.Errorf("Upsert(%q) expected error, got nil", tc.name)
			}
			if !tc.valid && err != nil && !strings.Contains(err.Error(), "invalid env var name") {
				t.Errorf("Upsert(%q) error = %q, want 'invalid env var name'", tc.name, err.Error())
			}
		})
	}
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func TestProjectEnvVarService_ReservedNames(t *testing.T) {
	svc, projectID := setupEnvVarSvc(t)

	reserved := []string{
		"NRFLO_PROJECT", "NRFLO_AGENT_TOKEN", "NRFLO_SDK_DIR", "NRFLO_HOME",
		"NRF_SESSION_ID", "NRF_WORKFLOW_INSTANCE_ID", "NRF_TRX", "NRF_SPAWNED",
		"NRF_CONTEXT_THRESHOLD", "NRF_MAX_CONTEXT", "CLAUDECODE", "PATH", "HOME",
	}

	for _, name := range reserved {
		name := name
		t.Run(name, func(t *testing.T) {
			_, err := svc.Upsert(projectID, name, "value")
			if err == nil {
				t.Errorf("Upsert(%q) expected error for reserved name, got nil", name)
				return
			}
			if !strings.Contains(err.Error(), "is reserved") {
				t.Errorf("Upsert(%q) error = %q, want 'is reserved'", name, err.Error())
			}
		})
	}
}

func TestProjectEnvVarService_ValueLengthBoundary(t *testing.T) {
	svc, projectID := setupEnvVarSvc(t)

	// 4096 bytes: OK
	if _, err := svc.Upsert(projectID, "OK_VAR", strings.Repeat("x", 4096)); err != nil {
		t.Errorf("Upsert() 4096-byte value error: %v (want nil)", err)
	}

	// 4097 bytes: rejected
	_, err := svc.Upsert(projectID, "BIG_VAR", strings.Repeat("x", 4097))
	if err == nil {
		t.Error("Upsert() 4097-byte value expected error, got nil")
	} else if !strings.Contains(err.Error(), "exceeds maximum length") {
		t.Errorf("Upsert() error = %q, want 'exceeds maximum length'", err.Error())
	}
}

func TestProjectEnvVarService_DeleteHappyPath(t *testing.T) {
	svc, projectID := setupEnvVarSvc(t)

	if _, err := svc.Upsert(projectID, "TO_DEL", "value"); err != nil {
		t.Fatalf("Upsert(): %v", err)
	}
	if err := svc.Delete(projectID, "TO_DEL"); err != nil {
		t.Fatalf("Delete(): %v", err)
	}

	list, err := svc.List(projectID)
	if err != nil {
		t.Fatalf("List(): %v", err)
	}
	if len(list) != 0 {
		t.Errorf("List() after Delete = %d items, want 0", len(list))
	}
}

func TestProjectEnvVarService_DeleteNotFound(t *testing.T) {
	svc, projectID := setupEnvVarSvc(t)

	err := svc.Delete(projectID, "MISSING")
	if err == nil {
		t.Error("Delete() expected error for missing var, got nil")
	}
}

func TestProjectEnvVarService_ListOrderedByName(t *testing.T) {
	svc, projectID := setupEnvVarSvc(t)

	for _, name := range []string{"ZEBRA_SVC", "ALPHA_SVC", "MANGO_SVC"} {
		if _, err := svc.Upsert(projectID, name, "v"); err != nil {
			t.Fatalf("Upsert(%s): %v", name, err)
		}
	}

	list, err := svc.List(projectID)
	if err != nil {
		t.Fatalf("List(): %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("List() = %d items, want 3", len(list))
	}
	if list[0].Name != "ALPHA_SVC" {
		t.Errorf("list[0].Name = %q, want ALPHA_SVC", list[0].Name)
	}
	if list[2].Name != "ZEBRA_SVC" {
		t.Errorf("list[2].Name = %q, want ZEBRA_SVC", list[2].Name)
	}
}
