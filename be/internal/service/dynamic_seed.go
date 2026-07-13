package service

import (
	"database/sql"
	"fmt"
	"time"

	"be/internal/clock"
	"be/internal/db"
)

// EnsureGlobalDynamicWorkflow idempotently create-if-absent seeds the bundled
// `dynamic` workflow under GlobalProjectID: a plan-driven workflow (fanout_template
// agent defs only, zero static/executable phases) that backs the
// dynamic_workflow/revise_plan/approve_plan tools when no specific workflow is
// named. Mirrors EnsureGlobalDeepResearch exactly (direct SQL, bypasses the
// agent-def service layer's model validation since it's shipped data); assumes
// the caller has already ensured the global project exists (EnsureGlobalDeepResearch
// runs first at startup — see cli/serve.go). Safe and cheap to call on every
// startup.
func EnsureGlobalDynamicWorkflow(pool *db.Pool, clk clock.Clock, rootPath string) error {
	now := clk.Now().UTC().Format(time.RFC3339Nano)

	if _, err := pool.Exec(
		`INSERT OR IGNORE INTO projects (id, name, root_path, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		GlobalProjectID, "Global Workflows", rootPath, now, now); err != nil {
		return fmt.Errorf("dynamic workflow seed: project: %w", err)
	}

	var existing string
	err := pool.QueryRow(
		`SELECT id FROM workflows WHERE project_id = ? AND id = ?`,
		GlobalProjectID, DynamicWorkflow).Scan(&existing)
	if err == nil {
		return nil // already seeded
	}
	if err != sql.ErrNoRows {
		return fmt.Errorf("dynamic workflow seed: probe: %w", err)
	}

	tx, err := pool.Begin()
	if err != nil {
		return fmt.Errorf("dynamic workflow seed: begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.Exec(
		`INSERT INTO workflows (id, project_id, description, scope_type, groups, close_ticket_on_complete, purge_on_completion, callable_as_subworkflow, is_global, finding_schemas, created_at, updated_at)
		 VALUES (?, ?, ?, 'project', '[]', 0, 0, 1, 1, '[]', ?, ?)`,
		DynamicWorkflow, GlobalProjectID, "Dynamically planned, on-demand multi-agent workflow", now, now); err != nil {
		return fmt.Errorf("dynamic workflow seed: workflow: %w", err)
	}

	for _, a := range dynAgents {
		if _, err := tx.Exec(
			`INSERT INTO agent_definitions (id, project_id, workflow_id, model, timeout, prompt, layer, execution_mode, tools, node_role, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, 0, 'cli_interactive', ?, 'fanout_template', ?, ?)`,
			a.ID, GlobalProjectID, DynamicWorkflow, a.Model, 30, a.Prompt, a.Tools, now, now); err != nil {
			return fmt.Errorf("dynamic workflow seed: agent %s: %w", a.ID, err)
		}
	}

	return tx.Commit()
}
