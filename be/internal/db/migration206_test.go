package db

import (
	"database/sql"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/golang-migrate/migrate/v4"
)

// TestMigration206_TemplateStepsCarryLengthBudgets verifies 000206 layered
// the length-budget guidance onto default_templates.steps (the setup-analyzer
// readonly template row) without changing the step_id sequence 000205 seeded.
func TestMigration206_TemplateStepsCarryLengthBudgets(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	var stepsJSON sql.NullString
	var readonly int
	err = pool.QueryRow(
		`SELECT steps, readonly FROM default_templates WHERE id = 'setup-analyzer'`,
	).Scan(&stepsJSON, &readonly)
	if err != nil {
		t.Fatalf("SELECT default_templates id=setup-analyzer: %v", err)
	}
	if readonly != 1 {
		t.Errorf("readonly = %d, want 1", readonly)
	}
	if !stepsJSON.Valid || stepsJSON.String == "" {
		t.Fatal("steps column is NULL/empty")
	}

	var steps []stepDef
	if err := json.Unmarshal([]byte(stepsJSON.String), &steps); err != nil {
		t.Fatalf("unmarshal steps: %v", err)
	}
	wantIDs := []string{"requirements-risks", "explore-backend", "explore-frontend", "cross-check"}
	if len(steps) != len(wantIDs) {
		t.Fatalf("len(steps) = %d, want %d", len(steps), len(wantIDs))
	}
	for i, want := range wantIDs {
		if steps[i].StepID != want {
			t.Errorf("steps[%d].StepID = %q, want %q", i, steps[i].StepID, want)
		}
	}

	// The length-budget guidance text added out-of-band (and now seeded by
	// 000206) must be present in the exploration steps' instructions.
	for _, id := range []string{"explore-backend", "explore-frontend"} {
		found := false
		for _, s := range steps {
			if s.StepID != id {
				continue
			}
			found = true
			if !strings.Contains(s.Instruction, "Findings are streamed") {
				t.Errorf("steps[%s].Instruction missing the length-budget guidance", id)
			}
		}
		if !found {
			t.Fatalf("step %q not found", id)
		}
	}
	for _, s := range steps {
		if s.StepID == "requirements-risks" && !strings.Contains(s.Instruction, "Keep `plan_risks` under") {
			t.Errorf("steps[requirements-risks].Instruction missing the plan_risks length budget")
		}
	}

	// No instruction may contain unexpanded template placeholder syntax
	// (mirrors migration205_test.go's equivalent check).
	for i, s := range steps {
		if strings.Contains(s.Instruction, "${") || strings.Contains(s.Instruction, "#{") {
			t.Errorf("steps[%d] (%s) instruction contains a template placeholder, want none: %q", i, s.StepID, s.Instruction)
		}
	}
}

// TestMigration206_LiveNrworkflowPlanDefStepsMatchTemplate verifies the
// second UPDATE re-copies the budgeted steps onto a pre-existing live
// nrworkflow/feature/plan def (as 000205 would have converted it to
// stepwise), so environments lacking the out-of-band edit converge; a
// same-id 'plan' def in another project must stay untouched (mirrors
// migration205_test.go's TestMigration205_StepwiseConversion_PreExistingRows
// seeding pattern).
func TestMigration206_LiveNrworkflowPlanDefStepsMatchTemplate(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "migration206.db")
	sqlDB, err := sql.Open("sqlite", buildDSN(dbPath))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })

	m := newMigrateInstance(t, sqlDB)
	if err := m.Migrate(205); err != nil {
		t.Fatalf("migrate to 205: %v", err)
	}

	now := "2026-01-01T00:00:00Z"
	seedProject := func(id string) {
		if _, err := sqlDB.Exec(`INSERT INTO projects (id, name, created_at, updated_at) VALUES (?, ?, ?, ?)`, id, id, now, now); err != nil {
			t.Fatalf("seed project %s: %v", id, err)
		}
	}
	seedWorkflow := func(projectID, workflowID string) {
		if _, err := sqlDB.Exec(`INSERT INTO workflows (project_id, id, description, scope_type, created_at, updated_at) VALUES (?, ?, '', 'ticket', ?, ?)`, projectID, workflowID, now, now); err != nil {
			t.Fatalf("seed workflow %s/%s: %v", projectID, workflowID, err)
		}
	}
	// A stepwise nrworkflow/feature/plan def (as 000205 would leave it,
	// pre-000206) that lacks the length-budget guidance text.
	seedStepwiseDef := func(projectID, workflowID, defID string) {
		if _, err := sqlDB.Exec(`INSERT INTO agent_definitions (id, project_id, workflow_id, model, timeout, prompt, prompt_mode, steps, created_at, updated_at)
			VALUES (?, ?, ?, 'sonnet', 20, 'original prompt', 'stepwise', '[]', ?, ?)`, defID, projectID, workflowID, now, now); err != nil {
			t.Fatalf("seed agent_definitions %s/%s/%s: %v", projectID, workflowID, defID, err)
		}
	}
	// A 'full' def must never be flipped/touched by 000206's guarded UPDATE.
	seedFullDef := func(projectID, workflowID, defID string) {
		if _, err := sqlDB.Exec(`INSERT INTO agent_definitions (id, project_id, workflow_id, model, timeout, prompt, created_at, updated_at)
			VALUES (?, ?, ?, 'sonnet', 20, 'original prompt', ?, ?)`, defID, projectID, workflowID, now, now); err != nil {
			t.Fatalf("seed agent_definitions %s/%s/%s: %v", projectID, workflowID, defID, err)
		}
	}

	seedProject("nrworkflow")
	seedWorkflow("nrworkflow", "feature")
	seedStepwiseDef("nrworkflow", "feature", "plan")

	seedProject("other")
	seedWorkflow("other", "feature")
	seedFullDef("other", "feature", "plan")

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		t.Fatalf("migrate up (remaining): %v", err)
	}

	pool, err := NewPoolPathExisting(dbPath, DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	var wantSteps string
	if err := pool.QueryRow(`SELECT steps FROM default_templates WHERE id = 'setup-analyzer'`).Scan(&wantSteps); err != nil {
		t.Fatalf("read template row: %v", err)
	}

	var steps sql.NullString
	if err := pool.QueryRow(`SELECT steps FROM agent_definitions WHERE project_id = 'nrworkflow' AND workflow_id = 'feature' AND id = 'plan'`).Scan(&steps); err != nil {
		t.Fatalf("read live plan def: %v", err)
	}
	if !steps.Valid || steps.String != wantSteps {
		t.Errorf("nrworkflow/feature/plan steps does not match the budgeted template row")
	}

	var otherSteps sql.NullString
	if err := pool.QueryRow(`SELECT steps FROM agent_definitions WHERE project_id = 'other' AND workflow_id = 'feature' AND id = 'plan'`).Scan(&otherSteps); err != nil {
		t.Fatalf("read other/feature/plan: %v", err)
	}
	if otherSteps.Valid {
		t.Errorf("other/feature/plan steps = %v, want untouched NULL ('full' def, unrelated project)", otherSteps.String)
	}
}

// TestMigration206_ReadonlyInvariantHoldsRepoWide re-verifies migration058's
// acceptance criterion after 000206's rewrite of the setup-analyzer row's
// steps column (template/default_template themselves are untouched by
// 000206, but the readonly invariant check must still hold repo-wide).
func TestMigration206_ReadonlyInvariantHoldsRepoWide(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	var mismatched int
	err = pool.QueryRow(
		"SELECT COUNT(*) FROM default_templates WHERE readonly = 1 AND template != default_template",
	).Scan(&mismatched)
	if err != nil {
		t.Fatalf("count mismatched rows: %v", err)
	}
	if mismatched != 0 {
		t.Errorf("readonly rows with template != default_template = %d, want 0", mismatched)
	}
}
