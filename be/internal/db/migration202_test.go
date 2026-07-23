package db

import (
	"fmt"
	"testing"
)

// TestMigration202_AgentDefinitionsPromptModeColumns verifies prompt_mode
// (NOT NULL, default 'full') and steps (nullable TEXT) were added to
// agent_definitions.
func TestMigration202_AgentDefinitionsPromptModeColumns(t *testing.T) {
	t.Parallel()
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	cols := tableColumns(t, pool, "agent_definitions")

	promptMode, ok := cols["prompt_mode"]
	if !ok {
		t.Fatal("prompt_mode column missing from agent_definitions; migration 000202 may not have run")
	}
	if promptMode.colType != "TEXT" {
		t.Errorf("prompt_mode column type = %q, want TEXT", promptMode.colType)
	}
	if promptMode.notNull != 1 {
		t.Errorf("prompt_mode notNull = %d, want 1", promptMode.notNull)
	}
	if got := fmt.Sprintf("%v", promptMode.dflt); got != "'full'" {
		t.Errorf("prompt_mode default = %q, want \"'full'\"", got)
	}

	steps, ok := cols["steps"]
	if !ok {
		t.Fatal("steps column missing from agent_definitions; migration 000202 may not have run")
	}
	if steps.colType != "TEXT" {
		t.Errorf("steps column type = %q, want TEXT", steps.colType)
	}
	if steps.notNull != 0 {
		t.Errorf("steps notNull = %d, want 0 (nullable)", steps.notNull)
	}
}

// TestMigration202_PromptModeCheckRejectsBogusValue verifies the CHECK
// constraint on prompt_mode rejects any value outside full|stepwise, on both
// UPDATE and a fresh INSERT.
func TestMigration202_PromptModeCheckRejectsBogusValue(t *testing.T) {
	t.Parallel()
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	seedProjectWorkflow(t, pool)

	if _, err := pool.Exec(`INSERT INTO agent_definitions
		(id, project_id, workflow_id, model, timeout, prompt, layer, execution_mode, tools, native_tools, sandbox, validation_commands, consultant, node_role, description, created_at, updated_at)
		VALUES ('agt202', 'p1', 'wf1', 'sonnet-5', 20, 'do work', 0, 'cli_interactive', '', '', '', '[]', 0, 'static', '', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("seed agent_definitions row: %v", err)
	}

	if _, err := pool.Exec(`UPDATE agent_definitions SET prompt_mode = 'bogus' WHERE id = 'agt202'`); err == nil {
		t.Error("UPDATE prompt_mode='bogus': expected CHECK constraint error, got nil")
	}

	cases := []struct {
		mode    string
		wantErr bool
	}{
		{"full", false},
		{"stepwise", false},
		{"bogus", true},
	}
	for i, tc := range cases {
		id := fmt.Sprintf("agt202-check-%d", i)
		_, err := pool.Exec(`INSERT INTO agent_definitions
			(id, project_id, workflow_id, model, timeout, prompt, layer, execution_mode, tools, native_tools, sandbox, validation_commands, consultant, node_role, description, prompt_mode, created_at, updated_at)
			VALUES (?, 'p1', 'wf1', 'sonnet-5', 20, 'do work', 0, 'cli_interactive', '', '', '', '[]', 0, 'static', '', ?, datetime('now'), datetime('now'))`,
			id, tc.mode)
		if tc.wantErr && err == nil {
			t.Errorf("INSERT prompt_mode=%q: expected CHECK constraint error, got nil", tc.mode)
		}
		if !tc.wantErr && err != nil {
			t.Errorf("INSERT prompt_mode=%q: unexpected error: %v", tc.mode, err)
		}
	}
}

// TestMigration202_AgentStepCursorsSchema verifies the agent_step_cursors
// table shape: composite PK, defaults, and column types/notNull.
func TestMigration202_AgentStepCursorsSchema(t *testing.T) {
	t.Parallel()
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	cols := tableColumns(t, pool, "agent_step_cursors")

	want := map[string]struct {
		colType string
		notNull int
	}{
		"workflow_instance_id": {"TEXT", 1},
		"node_id":              {"TEXT", 1},
		"steps_snapshot":       {"TEXT", 1},
		"revision":             {"INTEGER", 1},
		"current_index":        {"INTEGER", 1},
		"completed":            {"TEXT", 1},
		"created_at":           {"TEXT", 1},
		"updated_at":           {"TEXT", 1},
	}
	for name, w := range want {
		col, ok := cols[name]
		if !ok {
			t.Fatalf("%s column missing from agent_step_cursors; migration 000202 may not have run", name)
		}
		if col.colType != w.colType {
			t.Errorf("%s column type = %q, want %q", name, col.colType, w.colType)
		}
		if col.notNull != w.notNull {
			t.Errorf("%s column notNull = %d, want %d", name, col.notNull, w.notNull)
		}
	}

	if got := fmt.Sprintf("%v", cols["revision"].dflt); got != "1" {
		t.Errorf("revision default = %q, want \"1\"", got)
	}
	if got := fmt.Sprintf("%v", cols["current_index"].dflt); got != "0" {
		t.Errorf("current_index default = %q, want \"0\"", got)
	}
	if got := fmt.Sprintf("%v", cols["completed"].dflt); got != "'[]'" {
		t.Errorf("completed default = %q, want \"'[]'\"", got)
	}

	// Composite PK: PRAGMA table_info pk column (index 5) is 1-based rank
	// within the PK for both columns.
	rows, err := pool.Query("PRAGMA table_info(agent_step_cursors)")
	if err != nil {
		t.Fatalf("PRAGMA table_info: %v", err)
	}
	defer rows.Close()
	pkCols := map[string]int{}
	for rows.Next() {
		var cid, notNull, pk int
		var name, colType string
		var dflt interface{}
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dflt, &pk); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if pk > 0 {
			pkCols[name] = pk
		}
	}
	if len(pkCols) != 2 {
		t.Fatalf("agent_step_cursors PK columns = %v, want exactly 2 (workflow_instance_id, node_id)", pkCols)
	}
	if _, ok := pkCols["workflow_instance_id"]; !ok {
		t.Error("workflow_instance_id is not part of the PK")
	}
	if _, ok := pkCols["node_id"]; !ok {
		t.Error("node_id is not part of the PK")
	}
}

// TestMigration202_AgentStepCursorsCascadeDelete verifies deleting the
// referenced workflow_instances row cascades to delete its agent_step_cursors
// row (FK ON DELETE CASCADE).
func TestMigration202_AgentStepCursorsCascadeDelete(t *testing.T) {
	t.Parallel()
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	seedProjectWorkflow(t, pool)
	if _, err := pool.Exec(`INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, scope_type, status, retry_count, created_at, updated_at)
		VALUES ('wfi-cursor202', 'p1', '', 'wf1', 'ticket', 'active', 0, datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("insert workflow_instance: %v", err)
	}
	if _, err := pool.Exec(`INSERT INTO agent_step_cursors
		(workflow_instance_id, node_id, steps_snapshot, revision, current_index, completed, created_at, updated_at)
		VALUES ('wfi-cursor202', 'node-a', '[]', 1, 0, '[]', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("insert agent_step_cursors: %v", err)
	}

	var count int
	if err := pool.QueryRow(`SELECT COUNT(*) FROM agent_step_cursors WHERE workflow_instance_id = 'wfi-cursor202'`).Scan(&count); err != nil {
		t.Fatalf("count before delete: %v", err)
	}
	if count != 1 {
		t.Fatalf("count before delete = %d, want 1", count)
	}

	if _, err := pool.Exec(`DELETE FROM workflow_instances WHERE id = 'wfi-cursor202'`); err != nil {
		t.Fatalf("delete workflow_instance: %v", err)
	}

	if err := pool.QueryRow(`SELECT COUNT(*) FROM agent_step_cursors WHERE workflow_instance_id = 'wfi-cursor202'`).Scan(&count); err != nil {
		t.Fatalf("count after delete: %v", err)
	}
	if count != 0 {
		t.Errorf("count after workflow_instance delete = %d, want 0 (FK CASCADE)", count)
	}
}

// TestMigration202_AgentStepCursorsDuplicatePKRejected verifies a duplicate
// (workflow_instance_id, node_id) insert violates the composite PK.
func TestMigration202_AgentStepCursorsDuplicatePKRejected(t *testing.T) {
	t.Parallel()
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	seedProjectWorkflow(t, pool)
	if _, err := pool.Exec(`INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, scope_type, status, retry_count, created_at, updated_at)
		VALUES ('wfi-dup202', 'p1', '', 'wf1', 'ticket', 'active', 0, datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("insert workflow_instance: %v", err)
	}
	if _, err := pool.Exec(`INSERT INTO agent_step_cursors
		(workflow_instance_id, node_id, steps_snapshot, revision, current_index, completed, created_at, updated_at)
		VALUES ('wfi-dup202', 'node-a', '[]', 1, 0, '[]', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("first insert: %v", err)
	}

	if _, err := pool.Exec(`INSERT INTO agent_step_cursors
		(workflow_instance_id, node_id, steps_snapshot, revision, current_index, completed, created_at, updated_at)
		VALUES ('wfi-dup202', 'node-a', '[]', 1, 0, '[]', datetime('now'), datetime('now'))`); err == nil {
		t.Error("duplicate (workflow_instance_id, node_id) insert: expected PK violation, got nil")
	}
}
