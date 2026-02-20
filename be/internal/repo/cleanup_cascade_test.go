package repo

import (
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
)

// TestCleanupCascade verifies that deleting a workflow instance via cleanup
// automatically deletes its associated agent sessions due to ON DELETE CASCADE.
func TestCleanupCascade(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	pool, err := db.NewPoolPath(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	defer pool.Close()

	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer database.Close()

	// Create project
	_, err = pool.Exec(`INSERT INTO projects (id, name, root_path, created_at, updated_at) VALUES (?, ?, ?, datetime('now'), datetime('now'))`,
		"test-project", "Test Project", "/tmp/test")
	if err != nil {
		t.Fatalf("failed to create project: %v", err)
	}

	// Create workflows
	phasesJSON, _ := json.Marshal([]map[string]interface{}{
		{"agent": "test-agent", "layer": 0},
	})
	for i := 1; i <= 3; i++ {
		wfID := "test-workflow-" + string(rune(i+'0'))
		_, err = pool.Exec(`INSERT INTO workflows (id, project_id, description, scope_type, phases, created_at, updated_at) VALUES (?, ?, ?, ?, ?, datetime('now'), datetime('now'))`,
			wfID, "test-project", "Test Workflow", "ticket", string(phasesJSON))
		if err != nil {
			t.Fatalf("failed to create workflow %s: %v", wfID, err)
		}
	}

	wfiRepo := NewWorkflowInstanceRepo(pool, clock.Real())
	asRepo := NewAgentSessionRepo(database, clock.Real())

	findings, _ := json.Marshal(map[string]interface{}{})

	// Create 3 completed workflow instances with different timestamps
	now := time.Now().UTC()
	instances := []struct {
		id         string
		workflowID string
		ticketID   string
		offset     time.Duration
	}{
		{"wfi-old-1", "test-workflow-1", "TKT-1", -5 * time.Minute},
		{"wfi-mid-1", "test-workflow-2", "TKT-2", -3 * time.Minute},
		{"wfi-new-1", "test-workflow-3", "TKT-3", -1 * time.Minute},
	}

	for _, inst := range instances {
		updatedAt := now.Add(inst.offset).Format(time.RFC3339Nano)
		_, err = pool.Exec(`INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, scope_type, findings, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			inst.id, "test-project", inst.ticketID, inst.workflowID, model.WorkflowInstanceCompleted, "ticket", string(findings), updatedAt, updatedAt)
		if err != nil {
			t.Fatalf("failed to create workflow instance %s: %v", inst.id, err)
		}

		// Create 2 agent sessions for each workflow instance
		for j := 1; j <= 2; j++ {
			sessID := inst.id + "-sess-" + string(rune(j+'0'))
			s := &model.AgentSession{
				ID:                 sessID,
				ProjectID:          "test-project",
				TicketID:           inst.ticketID,
				WorkflowInstanceID: inst.id,
				Phase:              "phase1",
				AgentType:          "test-agent",
				ModelID:            sql.NullString{String: "sonnet", Valid: true},
				Status:             model.AgentSessionCompleted,
			}
			if err := asRepo.Create(s); err != nil {
				t.Fatalf("failed to create agent session %s: %v", sessID, err)
			}
		}
	}

	// Verify we have 3 workflow instances and 6 agent sessions
	var wfiCount, asCount int
	err = pool.QueryRow(`SELECT COUNT(*) FROM workflow_instances`).Scan(&wfiCount)
	if err != nil {
		t.Fatalf("failed to count workflow instances: %v", err)
	}
	if wfiCount != 3 {
		t.Fatalf("expected 3 workflow instances before cleanup, got %d", wfiCount)
	}

	err = database.QueryRow(`SELECT COUNT(*) FROM agent_sessions`).Scan(&asCount)
	if err != nil {
		t.Fatalf("failed to count agent sessions: %v", err)
	}
	if asCount != 6 {
		t.Fatalf("expected 6 agent sessions before cleanup, got %d", asCount)
	}

	// Run cleanup to keep only 2 workflow instances
	// This should delete wfi-old-1 and its 2 agent sessions via CASCADE
	deleted, err := wfiRepo.CleanupKeepLatest(2)
	if err != nil {
		t.Fatalf("CleanupKeepLatest failed: %v", err)
	}
	if deleted != 1 {
		t.Errorf("expected 1 workflow instance deleted, got %d", deleted)
	}

	// Verify 2 workflow instances remain
	err = pool.QueryRow(`SELECT COUNT(*) FROM workflow_instances`).Scan(&wfiCount)
	if err != nil {
		t.Fatalf("failed to count workflow instances after cleanup: %v", err)
	}
	if wfiCount != 2 {
		t.Errorf("expected 2 workflow instances after cleanup, got %d", wfiCount)
	}

	// Verify only 4 agent sessions remain (CASCADE deleted the 2 sessions from wfi-old-1)
	err = database.QueryRow(`SELECT COUNT(*) FROM agent_sessions`).Scan(&asCount)
	if err != nil {
		t.Fatalf("failed to count agent sessions after cleanup: %v", err)
	}
	if asCount != 4 {
		t.Errorf("expected 4 agent sessions after cleanup (CASCADE), got %d", asCount)
	}

	// Verify wfi-old-1 is deleted
	var exists bool
	err = pool.QueryRow(`SELECT EXISTS(SELECT 1 FROM workflow_instances WHERE id = ?)`, "wfi-old-1").Scan(&exists)
	if err != nil {
		t.Fatalf("failed to check wfi-old-1 existence: %v", err)
	}
	if exists {
		t.Error("expected wfi-old-1 to be deleted")
	}

	// Verify wfi-old-1's sessions are also deleted via CASCADE
	err = database.QueryRow(`SELECT EXISTS(SELECT 1 FROM agent_sessions WHERE id = ?)`, "wfi-old-1-sess-1").Scan(&exists)
	if err != nil {
		t.Fatalf("failed to check session existence: %v", err)
	}
	if exists {
		t.Error("expected wfi-old-1-sess-1 to be deleted (CASCADE)")
	}

	err = database.QueryRow(`SELECT EXISTS(SELECT 1 FROM agent_sessions WHERE id = ?)`, "wfi-old-1-sess-2").Scan(&exists)
	if err != nil {
		t.Fatalf("failed to check session existence: %v", err)
	}
	if exists {
		t.Error("expected wfi-old-1-sess-2 to be deleted (CASCADE)")
	}

	// Verify remaining workflow instances' sessions still exist
	for _, wfiID := range []string{"wfi-mid-1", "wfi-new-1"} {
		for j := 1; j <= 2; j++ {
			sessID := wfiID + "-sess-" + string(rune(j+'0'))
			err = database.QueryRow(`SELECT EXISTS(SELECT 1 FROM agent_sessions WHERE id = ?)`, sessID).Scan(&exists)
			if err != nil {
				t.Fatalf("failed to check session %s existence: %v", sessID, err)
			}
			if !exists {
				t.Errorf("expected %s to still exist", sessID)
			}
		}
	}
}

// TestAgentSessionCleanupIndependent verifies that agent sessions can be cleaned
// independently when a workflow instance has many sessions but the instance itself
// is not deleted.
func TestAgentSessionCleanupIndependent(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	pool, err := db.NewPoolPath(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	defer pool.Close()

	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer database.Close()

	// Create project
	_, err = pool.Exec(`INSERT INTO projects (id, name, root_path, created_at, updated_at) VALUES (?, ?, ?, datetime('now'), datetime('now'))`,
		"test-project", "Test Project", "/tmp/test")
	if err != nil {
		t.Fatalf("failed to create project: %v", err)
	}

	// Create workflow
	phasesJSON, _ := json.Marshal([]map[string]interface{}{
		{"agent": "test-agent", "layer": 0},
	})
	_, err = pool.Exec(`INSERT INTO workflows (id, project_id, description, scope_type, phases, created_at, updated_at) VALUES (?, ?, ?, ?, ?, datetime('now'), datetime('now'))`,
		"test-workflow", "test-project", "Test Workflow", "ticket", string(phasesJSON))
	if err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}

	wfiRepo := NewWorkflowInstanceRepo(pool, clock.Real())
	asRepo := NewAgentSessionRepo(database, clock.Real())

	findings, _ := json.Marshal(map[string]interface{}{})

	// Create 1 active workflow instance
	wi := &model.WorkflowInstance{
		ID:         "wfi-active",
		ProjectID:  "test-project",
		TicketID:   "TKT-1",
		WorkflowID: "test-workflow",
		ScopeType:  "ticket",
		Status:     model.WorkflowInstanceActive,
		Findings:   string(findings),
	}
	if err := wfiRepo.Create(wi); err != nil {
		t.Fatalf("failed to create workflow instance: %v", err)
	}

	// Create 5 completed agent sessions for this workflow instance
	now := time.Now().UTC()
	for i := 1; i <= 5; i++ {
		updatedAt := now.Add(time.Duration(-5+i) * time.Minute)
		sessID := "sess-" + string(rune(i+'0'))

		_, err = database.Exec(`
			INSERT INTO agent_sessions
			(id, project_id, ticket_id, workflow_instance_id, phase, agent_type, model_id, status, created_at, updated_at)
			VALUES (?, 'test-project', 'TKT-1', 'wfi-active', 'phase1', 'test-agent', 'sonnet', ?, ?, ?)`,
			sessID, model.AgentSessionCompleted, updatedAt.Format(time.RFC3339Nano), updatedAt.Format(time.RFC3339Nano))
		if err != nil {
			t.Fatalf("failed to create session %s: %v", sessID, err)
		}
	}

	// Verify we have 1 workflow instance and 5 agent sessions
	var wfiCount, asCount int
	err = pool.QueryRow(`SELECT COUNT(*) FROM workflow_instances`).Scan(&wfiCount)
	if err != nil {
		t.Fatalf("failed to count workflow instances: %v", err)
	}
	if wfiCount != 1 {
		t.Fatalf("expected 1 workflow instance, got %d", wfiCount)
	}

	err = database.QueryRow(`SELECT COUNT(*) FROM agent_sessions`).Scan(&asCount)
	if err != nil {
		t.Fatalf("failed to count agent sessions: %v", err)
	}
	if asCount != 5 {
		t.Fatalf("expected 5 agent sessions, got %d", asCount)
	}

	// Run agent session cleanup to keep only 2 latest
	deleted, err := asRepo.CleanupKeepLatest(2)
	if err != nil {
		t.Fatalf("AgentSession CleanupKeepLatest failed: %v", err)
	}
	if deleted != 3 {
		t.Errorf("expected 3 agent sessions deleted, got %d", deleted)
	}

	// Verify workflow instance still exists (should not be affected)
	err = pool.QueryRow(`SELECT COUNT(*) FROM workflow_instances`).Scan(&wfiCount)
	if err != nil {
		t.Fatalf("failed to count workflow instances after cleanup: %v", err)
	}
	if wfiCount != 1 {
		t.Errorf("expected 1 workflow instance after cleanup, got %d", wfiCount)
	}

	// Verify only 2 agent sessions remain
	err = database.QueryRow(`SELECT COUNT(*) FROM agent_sessions`).Scan(&asCount)
	if err != nil {
		t.Fatalf("failed to count agent sessions after cleanup: %v", err)
	}
	if asCount != 2 {
		t.Errorf("expected 2 agent sessions after cleanup, got %d", asCount)
	}

	// Verify the 2 most recent sessions remain
	for _, id := range []string{"sess-4", "sess-5"} {
		var exists bool
		err = database.QueryRow(`SELECT EXISTS(SELECT 1 FROM agent_sessions WHERE id = ?)`, id).Scan(&exists)
		if err != nil {
			t.Fatalf("failed to check session %s existence: %v", id, err)
		}
		if !exists {
			t.Errorf("expected %s to remain (most recent)", id)
		}
	}

	// Verify oldest sessions were deleted
	for _, id := range []string{"sess-1", "sess-2", "sess-3"} {
		var exists bool
		err = database.QueryRow(`SELECT EXISTS(SELECT 1 FROM agent_sessions WHERE id = ?)`, id).Scan(&exists)
		if err != nil {
			t.Fatalf("failed to check session %s existence: %v", id, err)
		}
		if exists {
			t.Errorf("expected %s to be deleted (oldest)", id)
		}
	}
}
