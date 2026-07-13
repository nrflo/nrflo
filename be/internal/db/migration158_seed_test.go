package db

import (
	"strings"
	"testing"
)

// TestMigration158_PlannerSystemAgentSeeded verifies the two planner
// system_agent_definitions rows (cli_interactive + api variants) are seeded.
func TestMigration158_PlannerSystemAgentSeeded(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	cases := []struct {
		id            string
		executionMode string
	}{
		{"planner-system", "cli_interactive"},
		{"planner-system-api", "api"},
	}
	for _, tc := range cases {
		var role, model, executionMode, tools string
		err := pool.QueryRow(`SELECT role, model, execution_mode, tools FROM system_agent_definitions WHERE id = ?`, tc.id).
			Scan(&role, &model, &executionMode, &tools)
		if err != nil {
			t.Fatalf("SELECT system_agent_definitions id=%s: %v", tc.id, err)
		}
		if role != "planner" {
			t.Errorf("id=%s role = %q, want %q", tc.id, role, "planner")
		}
		if executionMode != tc.executionMode {
			t.Errorf("id=%s execution_mode = %q, want %q", tc.id, executionMode, tc.executionMode)
		}
		if !strings.Contains(tools, "emit_findings") {
			t.Errorf("id=%s tools = %q, want it to contain %q", tc.id, tools, "emit_findings")
		}
	}
}

// TestMigration158_PlannerRoleExecutionModeUniqueIndex verifies the
// (role, execution_mode) UNIQUE index (migration 000063:14) is satisfied by
// the two seeded planner rows and actually enforces uniqueness against a
// colliding insert.
func TestMigration158_PlannerRoleExecutionModeUniqueIndex(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	var count int
	if err := pool.QueryRow(`SELECT COUNT(*) FROM system_agent_definitions WHERE role = 'planner'`).Scan(&count); err != nil {
		t.Fatalf("count planner rows: %v", err)
	}
	if count != 2 {
		t.Fatalf("planner system_agent_definitions rows = %d, want 2 (planner-system, planner-system-api)", count)
	}

	_, err = pool.Exec(`INSERT INTO system_agent_definitions (
		id, role, model, timeout, prompt, tools,
		stall_start_timeout_sec, stall_running_timeout_sec, execution_mode,
		created_at, updated_at
	) VALUES (
		'planner-system-dup', 'planner', 'sonnet', 10, 'x', 'emit_findings,agent_finished,agent_fail',
		60, 180, 'cli_interactive',
		datetime('now'), datetime('now')
	)`)
	if err == nil {
		t.Error("insert colliding (role='planner', execution_mode='cli_interactive') row: expected UNIQUE constraint error, got nil")
	}
}
