// Integration tests for skip-tags feature (migration 000030).
// This file covers schema validation and repo round-trip tests.
package integration

import (
	"testing"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"
)

// TestMigration030Schema verifies migration 000030 added the correct columns
// and the skipped status CHECK constraint is valid.
func TestMigration030Schema(t *testing.T) {
	env := NewTestEnv(t)

	t.Run("groups column in workflows", func(t *testing.T) {
		var count int
		if err := env.Pool.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('workflows') WHERE name = 'groups'`).Scan(&count); err != nil {
			t.Fatalf("schema query: %v", err)
		}
		if count != 1 {
			t.Errorf("groups column missing from workflows (count=%d)", count)
		}
	})

	t.Run("tag column in agent_definitions", func(t *testing.T) {
		var count int
		if err := env.Pool.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('agent_definitions') WHERE name = 'tag'`).Scan(&count); err != nil {
			t.Fatalf("schema query: %v", err)
		}
		if count != 1 {
			t.Errorf("tag column missing from agent_definitions (count=%d)", count)
		}
	})

	t.Run("skip_tags column in workflow_instances", func(t *testing.T) {
		var count int
		if err := env.Pool.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('workflow_instances') WHERE name = 'skip_tags'`).Scan(&count); err != nil {
			t.Fatalf("schema query: %v", err)
		}
		if count != 1 {
			t.Errorf("skip_tags column missing from workflow_instances (count=%d)", count)
		}
	})

	t.Run("foreign key integrity", func(t *testing.T) {
		var violations int
		if err := env.Pool.QueryRow(`SELECT COUNT(*) FROM pragma_foreign_key_check()`).Scan(&violations); err != nil {
			t.Fatalf("foreign_key_check: %v", err)
		}
		if violations != 0 {
			t.Errorf("expected 0 FK violations, found %d", violations)
		}
	})

	t.Run("skipped status accepted in agent_sessions", func(t *testing.T) {
		env.CreateTicket(t, "ST030-SKIP-SCHEMA", "Skipped status test")
		env.InitWorkflow(t, "ST030-SKIP-SCHEMA")
		wfiID := env.GetWorkflowInstanceID(t, "ST030-SKIP-SCHEMA", "test")

		now := env.Clock.Now().UTC().Format("2006-01-02T15:04:05.999999999Z07:00")
		_, err := env.Pool.Exec(`
			INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type,
				model_id, status, result, result_reason, pid, findings,
				context_left, ancestor_session_id, spawn_command, prompt_context,
				restart_count, started_at, ended_at, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, NULL, 'skipped', NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, 0, ?, NULL, ?, ?)`,
			"sess-skip-schema", env.ProjectID, "ST030-SKIP-SCHEMA", wfiID, "analyzer", "analyzer",
			now, now, now)
		if err != nil {
			t.Fatalf("INSERT with status='skipped' violated CHECK constraint: %v", err)
		}

		var status string
		if err := env.Pool.QueryRow(`SELECT status FROM agent_sessions WHERE id = 'sess-skip-schema'`).Scan(&status); err != nil {
			t.Fatalf("read skipped session: %v", err)
		}
		if status != "skipped" {
			t.Errorf("status = %q, want %q", status, "skipped")
		}
	})
}

// TestMigration030RepoRoundTrips verifies groups, tag, and skip_tags round-trip
// through their respective repo layers, all within a single TestEnv.
func TestMigration030RepoRoundTrips(t *testing.T) {
	env := NewTestEnv(t)

	// WorkflowRepo requires *db.DB, not *db.Pool — open a second connection.
	wfDB, err := db.OpenPathExisting(env.Pool.Path)
	if err != nil {
		t.Fatalf("OpenPathExisting: %v", err)
	}
	t.Cleanup(func() { wfDB.Close() })

	wfRepo := repo.NewWorkflowRepo(wfDB, clock.Real())
	agentRepo := repo.NewAgentDefinitionRepo(env.Pool, clock.Real())
	wfiRepo := repo.NewWorkflowInstanceRepo(env.Pool, clock.Real())

	t.Run("workflow groups persist", func(t *testing.T) {
		wf := &model.Workflow{
			ID:        "wf-grp-it",
			ProjectID: env.ProjectID,
			ScopeType: "ticket",
		}
		wf.SetGroups([]string{"be", "fe", "docs"})

		if err := wfRepo.Create(wf); err != nil {
			t.Fatalf("Create: %v", err)
		}
		got, err := wfRepo.Get(env.ProjectID, "wf-grp-it")
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		groups := got.GetGroups()
		if len(groups) != 3 || groups[0] != "be" || groups[1] != "fe" || groups[2] != "docs" {
			t.Errorf("GetGroups() = %v, want [be fe docs]", groups)
		}
	})

	t.Run("workflow groups update", func(t *testing.T) {
		wf := &model.Workflow{
			ID:        "wf-grp-upd-it",
			ProjectID: env.ProjectID,
			ScopeType: "ticket",
		}
		wf.SetGroups([]string{"be"})
		if err := wfRepo.Create(wf); err != nil {
			t.Fatalf("Create: %v", err)
		}

		newGroups := `["be","fe"]`
		if err := wfRepo.Update(env.ProjectID, "wf-grp-upd-it", &repo.WorkflowUpdateFields{Groups: &newGroups}); err != nil {
			t.Fatalf("Update: %v", err)
		}
		got, err := wfRepo.Get(env.ProjectID, "wf-grp-upd-it")
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if g := got.GetGroups(); len(g) != 2 || g[0] != "be" || g[1] != "fe" {
			t.Errorf("GetGroups() after update = %v, want [be fe]", g)
		}
	})

	t.Run("agent definition tag create get update", func(t *testing.T) {
		def := &model.AgentDefinition{
			ID:         "ag-tag-it",
			ProjectID:  env.ProjectID,
			WorkflowID: "test",
			Model:      "sonnet",
			Timeout:    20,
			Prompt:     "prompt",
			Tag:        "be",
		}
		if err := agentRepo.Create(def); err != nil {
			t.Fatalf("Create: %v", err)
		}
		got, err := agentRepo.Get(env.ProjectID, "test", "ag-tag-it")
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if got.Tag != "be" {
			t.Errorf("Tag = %q, want %q", got.Tag, "be")
		}

		newTag := "fe"
		if err := agentRepo.Update(env.ProjectID, "test", "ag-tag-it", &repo.AgentDefUpdateFields{Tag: &newTag}); err != nil {
			t.Fatalf("Update: %v", err)
		}
		got2, err := agentRepo.Get(env.ProjectID, "test", "ag-tag-it")
		if err != nil {
			t.Fatalf("Get after update: %v", err)
		}
		if got2.Tag != "fe" {
			t.Errorf("Tag after update = %q, want %q", got2.Tag, "fe")
		}
	})

	t.Run("workflow instance skip_tags and dedup", func(t *testing.T) {
		env.CreateTicket(t, "ST030-WFI-IT", "skip_tags it test")
		env.InitWorkflow(t, "ST030-WFI-IT")
		wfiID := env.GetWorkflowInstanceID(t, "ST030-WFI-IT", "test")

		wfi, err := wfiRepo.Get(wfiID)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if tags := wfi.GetSkipTags(); len(tags) != 0 {
			t.Errorf("initial skip_tags = %v, want []", tags)
		}

		// AddSkipTag with dedup, then persist
		wfi.AddSkipTag("be")
		wfi.AddSkipTag("fe")
		wfi.AddSkipTag("be") // duplicate — ignored by model

		if err := wfiRepo.UpdateSkipTags(wfiID, wfi.SkipTags); err != nil {
			t.Fatalf("UpdateSkipTags: %v", err)
		}

		got, err := wfiRepo.Get(wfiID)
		if err != nil {
			t.Fatalf("Get after update: %v", err)
		}
		if tags := got.GetSkipTags(); len(tags) != 2 || tags[0] != "be" || tags[1] != "fe" {
			t.Errorf("GetSkipTags() = %v, want [be fe]", tags)
		}
	})
}
