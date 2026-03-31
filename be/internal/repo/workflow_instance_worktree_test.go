package repo

import (
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
)

// makeWorktreeTestDB creates a fresh pool and inserts the necessary project + workflow records.
func makeWorktreeTestDB(t *testing.T, suffix string) (*WorkflowInstanceRepo, *db.Pool) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	pool, err := db.NewPoolPath(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	projectID := "proj-" + suffix
	workflowID := "wf-" + suffix
	_, err = pool.Exec(`INSERT INTO projects (id, name, root_path, created_at, updated_at)
		VALUES (?, ?, ?, datetime('now'), datetime('now'))`,
		projectID, "Test Project", "/tmp/test")
	if err != nil {
		t.Fatalf("failed to insert project: %v", err)
	}
	phasesJSON, _ := json.Marshal([]map[string]interface{}{{"agent": "test-agent", "layer": 0}})
	_, err = pool.Exec(`INSERT INTO workflows (id, project_id, description, scope_type, phases, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, datetime('now'), datetime('now'))`,
		workflowID, projectID, "Test Workflow", "ticket", string(phasesJSON))
	if err != nil {
		t.Fatalf("failed to insert workflow: %v", err)
	}

	return NewWorkflowInstanceRepo(pool, clock.Real()), pool
}

func makeTicketWFI(suffix string) *model.WorkflowInstance {
	return &model.WorkflowInstance{
		ID:         "wfi-" + suffix,
		ProjectID:  "proj-" + suffix,
		TicketID:   "tkt-" + suffix,
		WorkflowID: "wf-" + suffix,
		ScopeType:  "ticket",
		Status:     model.WorkflowInstanceActive,
		Findings:   "{}",
	}
}

// TestUpdateWorktree_SetsFields verifies that UpdateWorktree persists worktree_path and
// branch_name correctly and they are readable via Get.
func TestUpdateWorktree_SetsFields(t *testing.T) {
	r, _ := makeWorktreeTestDB(t, "wt1")
	wi := makeTicketWFI("wt1")
	if err := r.Create(wi); err != nil {
		t.Fatalf("Create: %v", err)
	}

	const path = "/tmp/nrworkflow/worktrees/tkt-wt1"
	const branch = "tkt-wt1"
	if err := r.UpdateWorktree(wi.ID, path, branch); err != nil {
		t.Fatalf("UpdateWorktree: %v", err)
	}

	got, err := r.Get(wi.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !got.WorktreePath.Valid {
		t.Error("WorktreePath.Valid should be true after UpdateWorktree")
	}
	if got.WorktreePath.String != path {
		t.Errorf("WorktreePath.String = %q, want %q", got.WorktreePath.String, path)
	}
	if !got.BranchName.Valid {
		t.Error("BranchName.Valid should be true after UpdateWorktree")
	}
	if got.BranchName.String != branch {
		t.Errorf("BranchName.String = %q, want %q", got.BranchName.String, branch)
	}
}

// TestUpdateWorktree_EmptyStrings_StoresNull verifies that empty strings result in NULL columns.
func TestUpdateWorktree_EmptyStrings_StoresNull(t *testing.T) {
	r, _ := makeWorktreeTestDB(t, "wt2")
	wi := makeTicketWFI("wt2")
	if err := r.Create(wi); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// First set real values, then clear them with empty strings.
	r.UpdateWorktree(wi.ID, "/tmp/nrworkflow/worktrees/tkt-wt2", "tkt-wt2")
	if err := r.UpdateWorktree(wi.ID, "", ""); err != nil {
		t.Fatalf("UpdateWorktree with empty strings: %v", err)
	}

	got, err := r.Get(wi.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.WorktreePath.Valid {
		t.Errorf("WorktreePath should be NULL for empty string, got %q", got.WorktreePath.String)
	}
	if got.BranchName.Valid {
		t.Errorf("BranchName should be NULL for empty string, got %q", got.BranchName.String)
	}
}

// TestUpdateWorktree_NonexistentID_ReturnsError verifies that updating a missing row returns an error.
func TestUpdateWorktree_NonexistentID_ReturnsError(t *testing.T) {
	r, _ := makeWorktreeTestDB(t, "wt3")
	err := r.UpdateWorktree("nonexistent-id", "/tmp/worktrees/x", "x")
	if err == nil {
		t.Error("expected error for nonexistent workflow instance ID, got nil")
	}
}

// TestCreate_NullWorktreeFields verifies that a newly created workflow instance
// (without worktree info) has NULL worktree_path and branch_name.
func TestCreate_NullWorktreeFields(t *testing.T) {
	r, _ := makeWorktreeTestDB(t, "wt4")
	wi := makeTicketWFI("wt4")
	if err := r.Create(wi); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := r.Get(wi.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.WorktreePath.Valid {
		t.Errorf("WorktreePath should be NULL for new instance, got %q", got.WorktreePath.String)
	}
	if got.BranchName.Valid {
		t.Errorf("BranchName should be NULL for new instance, got %q", got.BranchName.String)
	}
}

// TestCreate_WithWorktreeFields_Roundtrip verifies that a workflow instance created
// with WorktreePath and BranchName pre-set persists and reads back correctly.
func TestCreate_WithWorktreeFields_Roundtrip(t *testing.T) {
	r, _ := makeWorktreeTestDB(t, "wt5")
	wi := makeTicketWFI("wt5")
	wi.WorktreePath = sql.NullString{String: "/tmp/nrworkflow/worktrees/tkt-wt5", Valid: true}
	wi.BranchName = sql.NullString{String: "tkt-wt5", Valid: true}

	if err := r.Create(wi); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := r.Get(wi.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !got.WorktreePath.Valid || got.WorktreePath.String != "/tmp/nrworkflow/worktrees/tkt-wt5" {
		t.Errorf("WorktreePath = %v, want {String:/tmp/nrworkflow/worktrees/tkt-wt5, Valid:true}", got.WorktreePath)
	}
	if !got.BranchName.Valid || got.BranchName.String != "tkt-wt5" {
		t.Errorf("BranchName = %v, want {String:tkt-wt5, Valid:true}", got.BranchName)
	}
}
