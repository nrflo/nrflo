package orchestrator

import (
	"testing"

	"be/internal/clock"
	"be/internal/service"
	"be/internal/types"
)

// TestResolveWorkflowDef_GlobalFallback verifies that a workflow not defined
// under the selected project resolves from the reserved global project, that a
// locally-defined workflow resolves without fallback, and that an unknown
// workflow errors.
func TestResolveWorkflowDef_GlobalFallback(t *testing.T) {
	env := newTestEnv(t)
	now := clock.Real().Now().UTC().Format("2006-01-02T15:04:05.999999999Z07:00")

	// Seed the reserved global project + a global workflow with one agent.
	if _, err := env.pool.Exec(
		`INSERT INTO projects (id, name, root_path, created_at, updated_at) VALUES (?, 'Global', NULL, ?, ?)`,
		service.GlobalProjectID, now, now); err != nil {
		t.Fatalf("seed global project: %v", err)
	}
	wfSvc := service.NewWorkflowService(env.pool, clock.Real())
	if _, err := wfSvc.CreateWorkflowDef(service.GlobalProjectID, &types.WorkflowDefCreateRequest{
		ID:        "gwf",
		ScopeType: "project",
	}); err != nil {
		t.Fatalf("create global workflow: %v", err)
	}
	if _, err := env.pool.Exec(
		`INSERT INTO agent_definitions (id, project_id, workflow_id, model, timeout, prompt, layer, created_at, updated_at)
		 VALUES ('scope', ?, 'gwf', 'sonnet-5', 20, 'p', 0, ?, ?)`,
		service.GlobalProjectID, now, now); err != nil {
		t.Fatalf("seed global agent: %v", err)
	}

	// Global workflow resolved from a different real project -> falls back.
	wf, agents, defProj, err := env.orch.resolveWorkflowDef(env.pool, env.project, "gwf")
	if err != nil {
		t.Fatalf("resolveWorkflowDef(global): %v", err)
	}
	if defProj != service.GlobalProjectID {
		t.Errorf("defProjectID = %q, want %q", defProj, service.GlobalProjectID)
	}
	if wf == nil || wf.ID != "gwf" {
		t.Errorf("workflow = %+v, want id gwf", wf)
	}
	if len(agents) != 1 || agents[0].ID != "scope" {
		t.Errorf("agents = %+v, want [scope]", agents)
	}

	// Locally-defined workflow ("test" is seeded under env.project) resolves locally.
	_, _, defProj2, err := env.orch.resolveWorkflowDef(env.pool, env.project, "test")
	if err != nil {
		t.Fatalf("resolveWorkflowDef(local): %v", err)
	}
	if defProj2 != env.project {
		t.Errorf("local defProjectID = %q, want %q", defProj2, env.project)
	}

	// Unknown workflow -> error.
	if _, _, _, err := env.orch.resolveWorkflowDef(env.pool, env.project, "nope"); err == nil {
		t.Fatal("expected error for unknown workflow")
	}
}
