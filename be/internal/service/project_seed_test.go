package service

import (
	"path/filepath"
	"testing"

	"be/internal/clock"
	"be/internal/db"
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
		workflowID, defID, wantModel, wantEffort, wantTemplate string
	}{
		{"feature", "setup-analyzer", "sonnet-5", "low", "tier-t2-extractor"},
		{"feature", "test-writer", "sonnet-5", "medium", "tier-t1-executor"},
		{"feature", "implementor", "sonnet-5", "medium", "tier-t1-executor"},
		{"feature", "qa-verifier", "sonnet-5", "low", "tier-t2-extractor"},
		{"feature", "doc-updater", "haiku-4-5", "low", "tier-t1-executor"},
		{"bugfix", "setup-analyzer", "sonnet-5", "low", "tier-t2-extractor"},
		{"bugfix", "implementor", "sonnet-5", "medium", "tier-t1-executor"},
		{"bugfix", "qa-verifier", "sonnet-5", "low", "tier-t2-extractor"},
		{"docs", "setup-analyzer", "sonnet-5", "low", "tier-t2-extractor"},
		{"docs", "doc-updater", "haiku-4-5", "low", "tier-t1-executor"},
		{"refactor", "setup-analyzer", "sonnet-5", "low", "tier-t2-extractor"},
		{"refactor", "implementor", "sonnet-5", "medium", "tier-t1-executor"},
		{"refactor", "qa-verifier", "sonnet-5", "low", "tier-t2-extractor"},
	}
	for _, c := range cases {
		model, effort, template, _ := getAgentDefFields(t, pool, "born-tiered", c.workflowID, c.defID)
		if model != c.wantModel || effort != c.wantEffort || template != c.wantTemplate {
			t.Errorf("%s/%s = (%q, %q, %q), want (%q, %q, %q)",
				c.workflowID, c.defID, model, effort, template, c.wantModel, c.wantEffort, c.wantTemplate)
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
