package service

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/types"
)

// setupDeleteProjWFEnv creates an isolated DB with a project, workflow, and workflow instance.
// Returns the pool and a WorkflowService. The caller uses t.Cleanup via the function itself.
func setupDeleteProjWFEnv(t *testing.T, projectID, instanceID, status, scopeType string) (*db.Pool, *WorkflowService) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "wf_delete_test.db")
	if err := svcCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err = pool.Exec(
		`INSERT INTO projects (id, name, root_path, created_at, updated_at) VALUES (?, 'P', '/tmp', ?, ?)`,
		projectID, now, now,
	); err != nil {
		t.Fatalf("insert project %q: %v", projectID, err)
	}

	svc := NewWorkflowService(pool, clock.Real())

	phases := makePhases(t)
	if _, err = svc.CreateWorkflowDef(projectID, &types.WorkflowDefCreateRequest{
		ID:        "wf-delete-test",
		Phases:    phases,
		ScopeType: scopeType,
	}); err != nil {
		t.Fatalf("CreateWorkflowDef: %v", err)
	}

	if _, err = pool.Exec(
		`INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, scope_type, status, findings, retry_count, created_at, updated_at)
		 VALUES (?, ?, '', 'wf-delete-test', ?, ?, '{}', 0, ?, ?)`,
		instanceID, strings.ToLower(projectID), scopeType, status, now, now,
	); err != nil {
		t.Fatalf("insert workflow instance %q: %v", instanceID, err)
	}

	return pool, svc
}

// TestDeleteProjectWorkflowInstance_Statuses verifies deletable statuses succeed and active fails.
func TestDeleteProjectWorkflowInstance_Statuses(t *testing.T) {
	cases := []struct {
		status  string
		wantErr bool
		errFrag string
	}{
		{"completed", false, ""},
		{"project_completed", false, ""},
		{"failed", false, ""},
		{"active", true, "active"},
	}

	for _, tc := range cases {
		t.Run(tc.status, func(t *testing.T) {
			projID := "proj-" + tc.status
			instID := "inst-" + tc.status
			_, svc := setupDeleteProjWFEnv(t, projID, instID, tc.status, "project")

			err := svc.DeleteProjectWorkflowInstance(projID, instID)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("DeleteProjectWorkflowInstance(%q status) = nil, want error", tc.status)
				}
				if tc.errFrag != "" && !strings.Contains(err.Error(), tc.errFrag) {
					t.Errorf("error = %q, want to contain %q", err.Error(), tc.errFrag)
				}
			} else {
				if err != nil {
					t.Errorf("DeleteProjectWorkflowInstance(%q status) = %v, want nil", tc.status, err)
				}
			}
		})
	}
}

// TestDeleteProjectWorkflowInstance_NotFound verifies error when instance does not exist.
func TestDeleteProjectWorkflowInstance_NotFound(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "wf_delete_notfound.db")
	if err := svcCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	svc := NewWorkflowService(pool, clock.Real())
	err = svc.DeleteProjectWorkflowInstance("any-project", "nonexistent-instance")
	if err == nil {
		t.Fatal("expected error for nonexistent instance, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "not found")
	}
}

// TestDeleteProjectWorkflowInstance_WrongProject verifies 404-style error for wrong project ID.
func TestDeleteProjectWorkflowInstance_WrongProject(t *testing.T) {
	_, svc := setupDeleteProjWFEnv(t, "proj-real", "inst-wrong-proj", "completed", "project")

	err := svc.DeleteProjectWorkflowInstance("proj-other", "inst-wrong-proj")
	if err == nil {
		t.Fatal("expected error when projectID doesn't match, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "not found")
	}
}

// TestDeleteProjectWorkflowInstance_TicketScoped verifies error for ticket-scoped instance.
func TestDeleteProjectWorkflowInstance_TicketScoped(t *testing.T) {
	_, svc := setupDeleteProjWFEnv(t, "proj-tscoped", "inst-tscoped", "completed", "ticket")

	err := svc.DeleteProjectWorkflowInstance("proj-tscoped", "inst-tscoped")
	if err == nil {
		t.Fatal("expected error for ticket-scoped instance, got nil")
	}
	// Error should indicate not-project-scoped, not "not found".
	if strings.Contains(err.Error(), "not found") {
		t.Errorf("ticket-scoped error = %q, should not say 'not found'", err.Error())
	}
}

// TestDeleteProjectWorkflowInstance_CaseInsensitiveProjectID verifies project ID comparison is case-insensitive.
func TestDeleteProjectWorkflowInstance_CaseInsensitiveProjectID(t *testing.T) {
	// CreateWorkflowDef lowercases the project ID, so seed with lowercase too.
	_, svc := setupDeleteProjWFEnv(t, "proj-case", "inst-case", "completed", "project")

	// Provide uppercase project ID — EqualFold should match.
	if err := svc.DeleteProjectWorkflowInstance("PROJ-CASE", "inst-case"); err != nil {
		t.Errorf("case-insensitive match failed: %v", err)
	}
}

// TestDeleteProjectWorkflowInstance_AlreadyDeleted verifies second delete returns "not found".
func TestDeleteProjectWorkflowInstance_AlreadyDeleted(t *testing.T) {
	_, svc := setupDeleteProjWFEnv(t, "proj-twice", "inst-twice", "completed", "project")

	if err := svc.DeleteProjectWorkflowInstance("proj-twice", "inst-twice"); err != nil {
		t.Fatalf("first delete: %v", err)
	}

	err := svc.DeleteProjectWorkflowInstance("proj-twice", "inst-twice")
	if err == nil {
		t.Fatal("expected error on second delete, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("second delete error = %q, want to contain 'not found'", err.Error())
	}
}

// TestDeleteProjectWorkflowInstance_InstanceRemovedFromDB verifies the DB row is gone after delete.
func TestDeleteProjectWorkflowInstance_InstanceRemovedFromDB(t *testing.T) {
	pool, svc := setupDeleteProjWFEnv(t, "proj-dbcheck", "inst-dbcheck", "failed", "project")

	if err := svc.DeleteProjectWorkflowInstance("proj-dbcheck", "inst-dbcheck"); err != nil {
		t.Fatalf("delete: %v", err)
	}

	var count int
	if err := pool.QueryRow(`SELECT COUNT(*) FROM workflow_instances WHERE id = ?`, "inst-dbcheck").Scan(&count); err != nil {
		t.Fatalf("count query: %v", err)
	}
	if count != 0 {
		t.Errorf("workflow_instances count after delete = %d, want 0", count)
	}
}
