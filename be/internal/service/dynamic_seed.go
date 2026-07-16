package service

import (
	"database/sql"
	"fmt"
	"time"

	"be/internal/clock"
	"be/internal/db"
)

// EnsureGlobalDynamicWorkflow idempotently seeds the reserved GlobalProjectID
// and the bundled `dynamic` workflow: a plan-driven workflow (a
// node_role='fanout_template' template catalog plus one workflow-local
// node_role='planner' override — zero node_role='static' defs, so zero
// executable phases) that backs the dynamic_workflow/revise_plan/approve_plan
// tools when no specific workflow is named. The global project is a HIDDEN,
// RUNNABLE home for project-agnostic ("global") tools: ensured to exist with
// a non-empty root_path (backfilled even on existing installs) so the
// orchestrator can execute there when no real project is in scope. Writes via
// direct SQL on purpose (shipped definition data, no agent-def service-layer
// model validation). Safe and cheap to call on every startup.
func EnsureGlobalDynamicWorkflow(pool *db.Pool, clk clock.Clock, rootPath string) error {
	now := clk.Now().UTC().Format(time.RFC3339Nano)

	if _, err := pool.Exec(
		`INSERT OR IGNORE INTO projects (id, name, root_path, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		GlobalProjectID, "Global Workflows", rootPath, now, now); err != nil {
		return fmt.Errorf("dynamic workflow seed: project: %w", err)
	}
	if _, err := pool.Exec(
		`UPDATE projects SET root_path = ?, updated_at = ? WHERE id = ? AND (root_path IS NULL OR root_path = '')`,
		rootPath, now, GlobalProjectID); err != nil {
		return fmt.Errorf("dynamic workflow seed: project root: %w", err)
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
		 VALUES (?, ?, ?, 'project', '[]', 0, 0, 1, 1, ?, ?, ?)`,
		DynamicWorkflow, GlobalProjectID, "Dynamically planned, on-demand multi-agent workflow", dynFindingSchemas, now, now); err != nil {
		return fmt.Errorf("dynamic workflow seed: workflow: %w", err)
	}

	for _, a := range dynAgents {
		nodeRole := a.NodeRole
		if nodeRole == "" {
			nodeRole = "fanout_template"
		}
		if _, err := tx.Exec(
			`INSERT INTO agent_definitions (id, project_id, workflow_id, model, timeout, prompt, layer, execution_mode, tools, node_role, description, reasoning_effort, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, 0, 'cli_interactive', ?, ?, ?, NULLIF(?, ''), ?, ?)`,
			a.ID, GlobalProjectID, DynamicWorkflow, a.Model, 30, dynPrompt(a.ID), a.Tools, nodeRole, a.Description, a.ReasoningEffort, now, now); err != nil {
			return fmt.Errorf("dynamic workflow seed: agent %s: %w", a.ID, err)
		}
	}

	return tx.Commit()
}
