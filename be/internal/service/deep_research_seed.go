package service

import (
	"database/sql"
	"fmt"
	"time"

	"be/internal/clock"
	"be/internal/db"
)

// EnsureGlobalDeepResearch idempotently seeds the reserved GlobalProjectID
// project and the bundled deep-research workflow (definition + agents + finding
// schemas + a layer-2 quorum:2 policy). Create-if-absent: an existing
// deep-research workflow under GlobalProjectID is left untouched so admin edits
// survive restarts. Safe (and cheap) to call on every startup.
//
// It writes via direct SQL on purpose: the agent-def service rejects api-mode
// agents when api_mode_enabled is off, but the definition is just data and must
// seed regardless of the runtime api-mode toggle.
func EnsureGlobalDeepResearch(pool *db.Pool, clk clock.Clock) error {
	var existing string
	err := pool.QueryRow(
		`SELECT id FROM workflows WHERE project_id = ? AND id = ?`,
		GlobalProjectID, DeepResearchWorkflow).Scan(&existing)
	if err == nil {
		return nil // already seeded
	}
	if err != sql.ErrNoRows {
		return fmt.Errorf("deep-research seed: probe: %w", err)
	}

	now := clk.Now().UTC().Format(time.RFC3339Nano)
	tx, err := pool.Begin()
	if err != nil {
		return fmt.Errorf("deep-research seed: begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.Exec(
		`INSERT OR IGNORE INTO projects (id, name, root_path, created_at, updated_at) VALUES (?, ?, NULL, ?, ?)`,
		GlobalProjectID, "Global Workflows", now, now); err != nil {
		return fmt.Errorf("deep-research seed: project: %w", err)
	}

	if _, err := tx.Exec(
		`INSERT INTO workflows (id, project_id, description, scope_type, groups, close_ticket_on_complete, purge_on_completion, is_global, finding_schemas, created_at, updated_at)
		 VALUES (?, ?, ?, 'project', '[]', 0, 0, 1, ?, ?, ?)`,
		DeepResearchWorkflow, GlobalProjectID, "Multi-source, fact-checked web research", drFindingSchemas, now, now); err != nil {
		return fmt.Errorf("deep-research seed: workflow: %w", err)
	}

	for _, a := range drAgents {
		if _, err := tx.Exec(
			`INSERT INTO agent_definitions (id, project_id, workflow_id, model, timeout, prompt, layer, execution_mode, tools, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, 'api', ?, ?, ?)`,
			a.ID, GlobalProjectID, DeepResearchWorkflow, a.Model, 30, drPrompt(a.ID), a.Layer, a.Tools, now, now); err != nil {
			return fmt.Errorf("deep-research seed: agent %s: %w", a.ID, err)
		}
	}

	if _, err := tx.Exec(
		`INSERT INTO workflow_layer_policies (project_id, workflow_id, layer, pass_policy, created_at, updated_at)
		 VALUES (?, ?, 2, 'quorum:2', ?, ?)`,
		GlobalProjectID, DeepResearchWorkflow, now, now); err != nil {
		return fmt.Errorf("deep-research seed: layer policy: %w", err)
	}

	return tx.Commit()
}
