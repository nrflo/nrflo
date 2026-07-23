package service

import (
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/types"
)

func setupProjectSeedTestEnv(t *testing.T) (*ProjectService, *db.Pool) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "project_seed_test.db")
	if err := svcCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return NewProjectService(pool, clock.Real()), pool
}

// TestProjectCreate_BornTiered asserts a freshly created project's classic
// workflow phases (feature/bugfix/docs/refactor) are seeded directly at
// TierMap's recommended model/effort/system_template_id.
func TestProjectCreate_BornTiered(t *testing.T) {
	t.Parallel()
	svc, pool := setupProjectSeedTestEnv(t)

	if _, err := svc.Create("born-tiered", &types.ProjectCreateRequest{Name: "Born Tiered"}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	cases := []struct {
		workflowID, defID, wantTemplate, wantTools string
		wantTier                                   int
	}{
		{"feature", "setup-analyzer", "tier-t2-extractor", "delegate,get_delegation", 2},
		{"feature", "test-writer", "tier-t1-executor", "delegate,get_delegation", 3},
		{"feature", "implementor", "tier-t1-executor", "delegate,get_delegation", 3},
		{"feature", "qa-verifier", "tier-t2-extractor", "", 2},
		{"feature", "doc-updater", "tier-t1-executor", "", 1},
		{"bugfix", "setup-analyzer", "tier-t2-extractor", "delegate,get_delegation", 2},
		{"bugfix", "implementor", "tier-t1-executor", "delegate,get_delegation", 3},
		{"bugfix", "qa-verifier", "tier-t2-extractor", "", 2},
		{"docs", "setup-analyzer", "tier-t2-extractor", "delegate,get_delegation", 2},
		{"docs", "doc-updater", "tier-t1-executor", "", 1},
		{"refactor", "setup-analyzer", "tier-t2-extractor", "delegate,get_delegation", 2},
		{"refactor", "implementor", "tier-t1-executor", "delegate,get_delegation", 3},
		{"refactor", "qa-verifier", "tier-t2-extractor", "", 2},
	}
	for _, c := range cases {
		model, effort, template, _ := getAgentDefFields(t, pool, "born-tiered", c.workflowID, c.defID)
		if model != "" || effort != "" || template != c.wantTemplate {
			t.Errorf("%s/%s = (%q, %q, %q), want ('', '', %q)",
				c.workflowID, c.defID, model, effort, template, c.wantTemplate)
		}
		tier := getAgentDefTier(t, pool, "born-tiered", c.workflowID, c.defID)
		if tier == nil || *tier != c.wantTier {
			t.Errorf("%s/%s tier = %v, want %d", c.workflowID, c.defID, tier, c.wantTier)
		}
		tools := getAgentDefTools(t, pool, "born-tiered", c.workflowID, c.defID)
		if tools != c.wantTools {
			t.Errorf("%s/%s tools = %q, want %q", c.workflowID, c.defID, tools, c.wantTools)
		}
	}
}

// TestProjectCreate_HotfixImplementorUntouched asserts the seeded hotfix
// implementor gets the tier map's model but no forced effort/system_template_id.
func TestProjectCreate_HotfixImplementorUntouched(t *testing.T) {
	t.Parallel()
	svc, pool := setupProjectSeedTestEnv(t)

	if _, err := svc.Create("hotfix-seed", &types.ProjectCreateRequest{}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	model, effort, template, _ := getAgentDefFields(t, pool, "hotfix-seed", "hotfix", "implementor")
	if model != "sonnet-5" {
		t.Errorf("hotfix implementor model = %q, want sonnet-5 (TierMap implementor recommendation)", model)
	}
	if effort != "" {
		t.Errorf("hotfix implementor reasoning_effort = %q, want empty (not forced)", effort)
	}
	if template != "" {
		t.Errorf("hotfix implementor system_template_id = %q, want empty (not forced)", template)
	}
	if tools := getAgentDefTools(t, pool, "hotfix-seed", "hotfix", "implementor"); tools != "" {
		t.Errorf("hotfix implementor tools = %q, want empty (hotfix implementor excluded from delegation grant)", tools)
	}
	if tier := getAgentDefTier(t, pool, "hotfix-seed", "hotfix", "implementor"); tier != nil {
		t.Errorf("hotfix implementor tier = %v, want nil (untiered)", *tier)
	}
}

// TestProjectCreate_SeedsPromptsFromDefaultTemplates asserts each seeded
// classic phase's prompt is non-empty (loaded from default_templates).
func TestProjectCreate_SeedsPromptsFromDefaultTemplates(t *testing.T) {
	t.Parallel()
	svc, pool := setupProjectSeedTestEnv(t)

	if _, err := svc.Create("prompted", &types.ProjectCreateRequest{}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	var prompt string
	if err := pool.QueryRow(`SELECT prompt FROM agent_definitions WHERE project_id = 'prompted' AND workflow_id = 'feature' AND id = 'implementor'`).Scan(&prompt); err != nil {
		t.Fatalf("query prompt: %v", err)
	}
	if prompt == "" {
		t.Error("seeded implementor prompt is empty, want default_templates content")
	}
}

// getAgentDefPromptMode reads back prompt_mode/steps for one def row.
func getAgentDefPromptMode(t *testing.T, pool *db.Pool, projectID, workflowID, defID string) (promptMode string, steps sql.NullString) {
	t.Helper()
	if err := pool.QueryRow(`SELECT prompt_mode, steps FROM agent_definitions WHERE project_id = ? AND workflow_id = ? AND id = ?`,
		projectID, workflowID, defID,
	).Scan(&promptMode, &steps); err != nil {
		t.Fatalf("getAgentDefPromptMode(%s/%s/%s): %v", projectID, workflowID, defID, err)
	}
	return
}

// TestProjectCreate_SetupAnalyzerBornStepwise asserts a freshly created
// project's setup-analyzer role is seeded prompt_mode='stepwise' with a
// non-empty steps JSON that itself passes validateStepDefinitions (the real
// guard the CRUD path enforces), across every classic workflow that includes
// a setup-analyzer phase.
func TestProjectCreate_SetupAnalyzerBornStepwise(t *testing.T) {
	t.Parallel()
	svc, pool := setupProjectSeedTestEnv(t)

	if _, err := svc.Create("stepwise-seed", &types.ProjectCreateRequest{}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	for _, workflowID := range []string{"feature", "bugfix", "docs", "refactor"} {
		promptMode, steps := getAgentDefPromptMode(t, pool, "stepwise-seed", workflowID, "setup-analyzer")
		if promptMode != PromptModeStepwise {
			t.Errorf("%s/setup-analyzer prompt_mode = %q, want stepwise", workflowID, promptMode)
		}
		if !steps.Valid || steps.String == "" {
			t.Fatalf("%s/setup-analyzer steps = %v, want non-empty JSON", workflowID, steps)
		}
		var decoded []model.StepDefinition
		if err := json.Unmarshal([]byte(steps.String), &decoded); err != nil {
			t.Fatalf("%s/setup-analyzer: unmarshal steps: %v", workflowID, err)
		}
		if err := validateStepDefinitions(decoded); err != nil {
			t.Errorf("%s/setup-analyzer: seeded steps fail validateStepDefinitions: %v", workflowID, err)
		}
	}
}

// TestProjectCreate_OtherRolesBornFull asserts every non-setup-analyzer
// classic phase (and the hotfix implementor) is seeded prompt_mode='full'
// with no steps.
func TestProjectCreate_OtherRolesBornFull(t *testing.T) {
	t.Parallel()
	svc, pool := setupProjectSeedTestEnv(t)

	if _, err := svc.Create("full-seed", &types.ProjectCreateRequest{}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	cases := []struct{ workflowID, defID string }{
		{"feature", "test-writer"},
		{"feature", "implementor"},
		{"feature", "qa-verifier"},
		{"feature", "doc-updater"},
		{"bugfix", "implementor"},
		{"bugfix", "qa-verifier"},
		{"docs", "doc-updater"},
		{"refactor", "implementor"},
		{"refactor", "qa-verifier"},
		{"hotfix", "implementor"},
	}
	for _, c := range cases {
		promptMode, steps := getAgentDefPromptMode(t, pool, "full-seed", c.workflowID, c.defID)
		if promptMode != PromptModeFull {
			t.Errorf("%s/%s prompt_mode = %q, want full", c.workflowID, c.defID, promptMode)
		}
		if steps.Valid {
			t.Errorf("%s/%s steps = %q, want NULL", c.workflowID, c.defID, steps.String)
		}
	}
}
