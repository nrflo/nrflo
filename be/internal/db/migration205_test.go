package db

import (
	"database/sql"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/golang-migrate/migrate/v4"
)

// stepDef mirrors model.StepDefinition's JSON shape locally (this package
// cannot import be/internal/model without a cycle risk in db tests, and the
// existing migration test files decode ad hoc structs rather than import
// model — mirrors migration203_test.go's approach of asserting on raw JSON).
type stepDef struct {
	StepID           string `json:"step_id"`
	Title            string `json:"title"`
	Instruction      string `json:"instruction"`
	RequiredFindings []struct {
		Key    string `json:"key"`
		Schema string `json:"schema"`
	} `json:"required_findings"`
	RotationAllowed bool `json:"rotation_allowed"`
	PathOverlap     *struct {
		Left  []string `json:"left"`
		Right []string `json:"right"`
	} `json:"path_overlap"`
}

// TestMigration205_TemplateStepsShape verifies the readonly setup-analyzer
// template row's steps JSON decodes to the exact 4-step pilot shape.
func TestMigration205_TemplateStepsShape(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	var template, defaultTemplate string
	var stepsJSON sql.NullString
	var readonly int
	err = pool.QueryRow(
		`SELECT template, default_template, steps, readonly FROM default_templates WHERE id = 'setup-analyzer'`,
	).Scan(&template, &defaultTemplate, &stepsJSON, &readonly)
	if err != nil {
		t.Fatalf("SELECT default_templates id=setup-analyzer: %v", err)
	}
	if readonly != 1 {
		t.Errorf("readonly = %d, want 1", readonly)
	}
	if template != defaultTemplate {
		t.Errorf("template != default_template (readonly invariant violated):\ntemplate=%q\ndefault_template=%q", template, defaultTemplate)
	}
	for _, anchor := range []string{"${TICKET_TITLE}", "${TICKET_DESCRIPTION}"} {
		if !strings.Contains(template, anchor) {
			t.Errorf("anchor prompt missing %q", anchor)
		}
	}

	if !stepsJSON.Valid || stepsJSON.String == "" {
		t.Fatal("steps column is NULL/empty, want the 4-step pilot JSON")
	}
	var steps []stepDef
	if err := json.Unmarshal([]byte(stepsJSON.String), &steps); err != nil {
		t.Fatalf("unmarshal steps: %v", err)
	}
	if len(steps) != 4 {
		t.Fatalf("len(steps) = %d, want 4", len(steps))
	}

	wantIDs := []string{"requirements-risks", "explore-backend", "explore-frontend", "cross-check"}
	for i, want := range wantIDs {
		if steps[i].StepID != want {
			t.Errorf("steps[%d].StepID = %q, want %q", i, steps[i].StepID, want)
		}
	}
	wantRotation := []bool{true, true, true, false}
	for i, want := range wantRotation {
		if steps[i].RotationAllowed != want {
			t.Errorf("steps[%d].RotationAllowed = %v, want %v", i, steps[i].RotationAllowed, want)
		}
	}

	// No instruction may contain template placeholder syntax — spawner
	// appends step instructions AFTER expansion, so ${...}/#{...} inside an
	// instruction would be echoed verbatim to the agent, never expanded.
	for i, s := range steps {
		if strings.Contains(s.Instruction, "${") || strings.Contains(s.Instruction, "#{") {
			t.Errorf("steps[%d] (%s) instruction contains a template placeholder, want none: %q", i, s.StepID, s.Instruction)
		}
	}

	// step 1: plan_risks/nonempty_text.
	assertRequiredFindings(t, steps[0], map[string]string{"plan_risks": "nonempty_text"})

	// step 2 (backend) and step 3 (frontend): six be_*/fe_* keys each.
	wantBE := map[string]string{
		"be_plan_summary":         "nonempty_text",
		"be_files_to_modify":      "json_array_path_change",
		"be_files_to_create":      "json_array_path_change",
		"be_implementation_steps": "ordered_lines",
		"be_patterns_to_follow":   "nonempty_text",
		"be_testing_notes":        "nonempty_text",
	}
	wantFE := map[string]string{
		"fe_plan_summary":         "nonempty_text",
		"fe_files_to_modify":      "json_array_path_change",
		"fe_files_to_create":      "json_array_path_change",
		"fe_implementation_steps": "ordered_lines",
		"fe_patterns_to_follow":   "nonempty_text",
		"fe_testing_notes":        "nonempty_text",
	}
	assertRequiredFindings(t, steps[1], wantBE)
	assertRequiredFindings(t, steps[2], wantFE)

	// step 4: plan_cross_check + path_overlap gate.
	assertRequiredFindings(t, steps[3], map[string]string{"plan_cross_check": "nonempty_text"})
	overlap := steps[3].PathOverlap
	if overlap == nil {
		t.Fatal("steps[3].PathOverlap = nil, want the be/fe file-ownership gate")
	}
	assertStringSlice(t, "path_overlap.left", overlap.Left, []string{"be_files_to_modify", "be_files_to_create"})
	assertStringSlice(t, "path_overlap.right", overlap.Right, []string{"fe_files_to_modify", "fe_files_to_create"})
}

func assertRequiredFindings(t *testing.T, s stepDef, want map[string]string) {
	t.Helper()
	if len(s.RequiredFindings) != len(want) {
		t.Errorf("%s: len(required_findings) = %d, want %d", s.StepID, len(s.RequiredFindings), len(want))
	}
	got := make(map[string]string, len(s.RequiredFindings))
	for _, f := range s.RequiredFindings {
		got[f.Key] = f.Schema
	}
	for key, schema := range want {
		if got[key] != schema {
			t.Errorf("%s: required_findings[%q].schema = %q, want %q", s.StepID, key, got[key], schema)
		}
	}
}

func assertStringSlice(t *testing.T, label string, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s = %v, want %v", label, got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("%s[%d] = %q, want %q", label, i, got[i], w)
		}
	}
}

// TestMigration205_ReadonlyInvariantHoldsRepoWide re-verifies migration058's
// acceptance criterion after 000205's rewrite of the setup-analyzer row.
func TestMigration205_ReadonlyInvariantHoldsRepoWide(t *testing.T) {
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

// TestMigration205_StepwiseConversion_PreExistingRows verifies the
// data-migration behavior against rows that existed before 000205 ran: the
// live nrworkflow/feature/plan def flips to stepwise with prompt+steps
// copied from the template row, while a same-id 'plan' def in another
// project and any other agent_definitions row are left untouched.
func TestMigration205_StepwiseConversion_PreExistingRows(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "migration205.db")
	sqlDB, err := sql.Open("sqlite", buildDSN(dbPath))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })

	m := newMigrateInstance(t, sqlDB)
	if err := m.Migrate(204); err != nil {
		t.Fatalf("migrate to 204: %v", err)
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
	seedDef := func(projectID, workflowID, defID string) {
		if _, err := sqlDB.Exec(`INSERT INTO agent_definitions (id, project_id, workflow_id, model, timeout, prompt, created_at, updated_at)
			VALUES (?, ?, ?, 'sonnet', 20, 'original prompt', ?, ?)`, defID, projectID, workflowID, now, now); err != nil {
			t.Fatalf("seed agent_definitions %s/%s/%s: %v", projectID, workflowID, defID, err)
		}
	}

	// The live row 000205 must convert.
	seedProject("nrworkflow")
	seedWorkflow("nrworkflow", "feature")
	seedDef("nrworkflow", "feature", "plan")

	// A same-id 'plan' def in a different project must stay untouched.
	seedProject("other")
	seedWorkflow("other", "feature")
	seedDef("other", "feature", "plan")

	// A setup-analyzer agent_definitions row (distinct from the
	// default_templates seed) must also stay untouched — 000205 only
	// touches nrworkflow/feature/plan.
	seedDef("other", "feature", "setup-analyzer")

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		t.Fatalf("migrate up (remaining): %v", err)
	}

	sqlxPool, err := NewPoolPathExisting(dbPath, DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(func() { sqlxPool.Close() })

	var wantPrompt, wantSteps string
	if err := sqlxPool.QueryRow(`SELECT template, steps FROM default_templates WHERE id = 'setup-analyzer'`).Scan(&wantPrompt, &wantSteps); err != nil {
		t.Fatalf("read template row: %v", err)
	}

	var promptMode, prompt string
	var steps sql.NullString
	if err := sqlxPool.QueryRow(`SELECT prompt_mode, prompt, steps FROM agent_definitions WHERE project_id = 'nrworkflow' AND workflow_id = 'feature' AND id = 'plan'`).Scan(&promptMode, &prompt, &steps); err != nil {
		t.Fatalf("read live plan def: %v", err)
	}
	if promptMode != "stepwise" {
		t.Errorf("nrworkflow/feature/plan prompt_mode = %q, want stepwise", promptMode)
	}
	if prompt != wantPrompt {
		t.Errorf("nrworkflow/feature/plan prompt does not match template row")
	}
	if !steps.Valid || steps.String != wantSteps {
		t.Errorf("nrworkflow/feature/plan steps does not match template row")
	}

	var otherPromptMode string
	var otherSteps sql.NullString
	if err := sqlxPool.QueryRow(`SELECT prompt_mode, steps FROM agent_definitions WHERE project_id = 'other' AND workflow_id = 'feature' AND id = 'plan'`).Scan(&otherPromptMode, &otherSteps); err != nil {
		t.Fatalf("read other/feature/plan: %v", err)
	}
	if otherPromptMode != "full" || otherSteps.Valid {
		t.Errorf("other/feature/plan = (%q, valid=%v), want ('full', NULL) untouched", otherPromptMode, otherSteps.Valid)
	}

	var otherSAPromptMode string
	var otherSASteps sql.NullString
	if err := sqlxPool.QueryRow(`SELECT prompt_mode, steps FROM agent_definitions WHERE project_id = 'other' AND workflow_id = 'feature' AND id = 'setup-analyzer'`).Scan(&otherSAPromptMode, &otherSASteps); err != nil {
		t.Fatalf("read other/feature/setup-analyzer: %v", err)
	}
	if otherSAPromptMode != "full" || otherSASteps.Valid {
		t.Errorf("other/feature/setup-analyzer = (%q, valid=%v), want ('full', NULL) untouched", otherSAPromptMode, otherSASteps.Valid)
	}

	var stepwiseCount int
	if err := sqlxPool.QueryRow(`SELECT COUNT(*) FROM agent_definitions WHERE prompt_mode = 'stepwise'`).Scan(&stepwiseCount); err != nil {
		t.Fatalf("count stepwise defs: %v", err)
	}
	if stepwiseCount != 1 {
		t.Errorf("COUNT(prompt_mode='stepwise') = %d, want 1", stepwiseCount)
	}
}
