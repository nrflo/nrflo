package service

import (
	"database/sql"
	"fmt"
	"time"

	"be/internal/clock"
	"be/internal/db"
)

// EnsureGlobalDeepResearch idempotently seeds the reserved GlobalProjectID and
// the bundled deep-research workflow. The global project is a HIDDEN, RUNNABLE
// home for project-agnostic ("global") tools: it is ensured to exist with a
// non-empty root_path (rootPath) so the orchestrator can execute there when no
// real project is in scope. The root_path is backfilled even on existing
// installs (where the project predates being runnable). The deep-research
// workflow itself is create-if-absent so admin edits survive restarts. Safe and
// cheap to call on every startup.
//
// It writes via direct SQL on purpose: it seeds shipped definition data at
// startup without the agent-def service layer (and its model-validation
// dependencies), so the bundled workflow is present regardless of runtime
// configuration. The agents run as cli_interactive (the claude/codex CLIs
// self-authenticate), so the workflow needs no server-side API credential.
func EnsureGlobalDeepResearch(pool *db.Pool, clk clock.Clock, rootPath string) error {
	now := clk.Now().UTC().Format(time.RFC3339Nano)

	// Ensure the hidden global project exists AND is runnable (has a root_path).
	// Runs unconditionally so installs created before it was runnable get the
	// root_path backfilled.
	if _, err := pool.Exec(
		`INSERT OR IGNORE INTO projects (id, name, root_path, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		GlobalProjectID, "Global Workflows", rootPath, now, now); err != nil {
		return fmt.Errorf("deep-research seed: project: %w", err)
	}
	if _, err := pool.Exec(
		`UPDATE projects SET root_path = ?, updated_at = ? WHERE id = ? AND (root_path IS NULL OR root_path = '')`,
		rootPath, now, GlobalProjectID); err != nil {
		return fmt.Errorf("deep-research seed: project root: %w", err)
	}

	// Create-if-absent the deep-research workflow.
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

	tx, err := pool.Begin()
	if err != nil {
		return fmt.Errorf("deep-research seed: begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.Exec(
		`INSERT INTO workflows (id, project_id, description, scope_type, groups, close_ticket_on_complete, purge_on_completion, callable_as_subworkflow, is_global, finding_schemas, created_at, updated_at)
		 VALUES (?, ?, ?, 'project', '[]', 0, 0, 1, 1, ?, ?, ?)`,
		DeepResearchWorkflow, GlobalProjectID, "Multi-source, fact-checked web research", drFindingSchemas, now, now); err != nil {
		return fmt.Errorf("deep-research seed: workflow: %w", err)
	}

	for _, a := range drAgents {
		if _, err := tx.Exec(
			`INSERT INTO agent_definitions (id, project_id, workflow_id, model, timeout, prompt, layer, execution_mode, tools, reasoning_effort, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, 'cli_interactive', ?, NULLIF(?, ''), ?, ?)`,
			a.ID, GlobalProjectID, DeepResearchWorkflow, a.Model, 30, drPrompt(a.ID), a.Layer, a.Tools, a.ReasoningEffort, now, now); err != nil {
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
