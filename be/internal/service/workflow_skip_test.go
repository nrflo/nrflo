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

// setupSkipTestEnv creates a DB with project, workflow (with groups), and a workflow instance.
// Returns pool, service, and wfiID.
func setupSkipTestEnv(t *testing.T) (*db.Pool, *WorkflowService, string) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "skip_test.db")
	if err := svcCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err = pool.Exec(`INSERT INTO projects (id, name, root_path, created_at, updated_at) VALUES (?, 'P', '/tmp', ?, ?)`,
		"proj1", now, now); err != nil {
		t.Fatalf("project insert: %v", err)
	}

	svc := NewWorkflowService(pool, clock.Real())

	// Create workflow with groups
	_, err = svc.CreateWorkflowDef("proj1", &types.WorkflowDefCreateRequest{
		ID:     "wf1",
		Groups: []string{"be", "fe", "docs"},
	})
	if err != nil {
		t.Fatalf("CreateWorkflowDef: %v", err)
	}

	// Create workflow instance directly (no ticket needed for service-level test)
	wfiID := "wfi-skip-test"
	if _, err = pool.Exec(`
		INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, scope_type, status, findings, retry_count, created_at, updated_at)
		VALUES (?, 'proj1', 'ticket-1', 'wf1', 'ticket', 'active', '{}', 0, ?, ?)`,
		wfiID, now, now); err != nil {
		t.Fatalf("workflow_instance insert: %v", err)
	}

	return pool, svc, wfiID
}

// getSkipTags reads the skip_tags column from DB for a given workflow instance.
func getSkipTags(t *testing.T, pool *db.Pool, wfiID string) string {
	t.Helper()
	var skipTags string
	if err := pool.QueryRow(`SELECT COALESCE(skip_tags, '[]') FROM workflow_instances WHERE id = ?`, wfiID).Scan(&skipTags); err != nil {
		t.Fatalf("getSkipTags(%s): %v", wfiID, err)
	}
	return skipTags
}

// TestAddSkipTag_HappyPath verifies a valid tag is added to skip_tags.
func TestAddSkipTag_HappyPath(t *testing.T) {
	pool, svc, wfiID := setupSkipTestEnv(t)

	projectID, ticketID, workflowID, err := svc.AddSkipTag(wfiID, "be")
	if err != nil {
		t.Fatalf("AddSkipTag: %v", err)
	}
	if projectID != "proj1" {
		t.Errorf("projectID = %q, want %q", projectID, "proj1")
	}
	if ticketID != "ticket-1" {
		t.Errorf("ticketID = %q, want %q", ticketID, "ticket-1")
	}
	if workflowID != "wf1" {
		t.Errorf("workflowID = %q, want %q", workflowID, "wf1")
	}

	// Verify persisted in DB
	skipTags := getSkipTags(t, pool, wfiID)
	if skipTags != `["be"]` {
		t.Errorf("skip_tags in DB = %q, want %q", skipTags, `["be"]`)
	}
}

// TestAddSkipTag_Idempotent verifies adding the same tag twice does not duplicate.
func TestAddSkipTag_Idempotent(t *testing.T) {
	pool, svc, wfiID := setupSkipTestEnv(t)

	if _, _, _, err := svc.AddSkipTag(wfiID, "be"); err != nil {
		t.Fatalf("first AddSkipTag: %v", err)
	}
	if _, _, _, err := svc.AddSkipTag(wfiID, "be"); err != nil {
		t.Fatalf("second AddSkipTag (idempotent): %v", err)
	}

	skipTags := getSkipTags(t, pool, wfiID)
	if skipTags != `["be"]` {
		t.Errorf("idempotent skip_tags = %q, want %q", skipTags, `["be"]`)
	}
}

// TestAddSkipTag_MultipleTags verifies multiple distinct tags accumulate.
func TestAddSkipTag_MultipleTags(t *testing.T) {
	pool, svc, wfiID := setupSkipTestEnv(t)

	for _, tag := range []string{"be", "fe", "docs"} {
		if _, _, _, err := svc.AddSkipTag(wfiID, tag); err != nil {
			t.Fatalf("AddSkipTag(%s): %v", tag, err)
		}
	}

	skipTags := getSkipTags(t, pool, wfiID)
	// All three should be present in the JSON array
	for _, want := range []string{`"be"`, `"fe"`, `"docs"`} {
		if !strings.Contains(skipTags, want) {
			t.Errorf("skip_tags = %q, expected to contain %s", skipTags, want)
		}
	}
}

// TestAddSkipTag_TagNotInGroups verifies an error is returned for unknown tags.
func TestAddSkipTag_TagNotInGroups(t *testing.T) {
	_, svc, wfiID := setupSkipTestEnv(t)

	_, _, _, err := svc.AddSkipTag(wfiID, "nonexistent")
	if err == nil {
		t.Fatal("expected error for tag not in groups, got nil")
	}
}

// TestAddSkipTag_InstanceNotFound verifies an error is returned for nonexistent instances.
func TestAddSkipTag_InstanceNotFound(t *testing.T) {
	_, svc, _ := setupSkipTestEnv(t)

	_, _, _, err := svc.AddSkipTag("nonexistent-instance-id", "be")
	if err == nil {
		t.Fatal("expected error for nonexistent instance, got nil")
	}
}

// TestAddSkipTag_WorkflowNoGroups verifies error when workflow has no groups.
func TestAddSkipTag_WorkflowNoGroups(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "skip_nogroups_test.db")
	if err := svcCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err = pool.Exec(`INSERT INTO projects (id, name, root_path, created_at, updated_at) VALUES (?, 'P', '/tmp', ?, ?)`,
		"proj2", now, now); err != nil {
		t.Fatalf("project insert: %v", err)
	}

	svc := NewWorkflowService(pool, clock.Real())
	if _, err = svc.CreateWorkflowDef("proj2", &types.WorkflowDefCreateRequest{
		ID: "wf-nogroups",
		// No groups
	}); err != nil {
		t.Fatalf("CreateWorkflowDef: %v", err)
	}

	wfiID := "wfi-nogroups"
	if _, err = pool.Exec(`
		INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, scope_type, status, findings, retry_count, created_at, updated_at)
		VALUES (?, 'proj2', 't1', 'wf-nogroups', 'ticket', 'active', '{}', 0, ?, ?)`,
		wfiID, now, now); err != nil {
		t.Fatalf("workflow_instance insert: %v", err)
	}

	_, _, _, err = svc.AddSkipTag(wfiID, "be")
	if err == nil {
		t.Fatal("expected error when workflow has no groups, got nil")
	}
}

