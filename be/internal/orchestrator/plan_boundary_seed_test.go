package orchestrator

import (
	"context"
	"encoding/json"
	"testing"

	"be/internal/clock"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/service"
)

// A caller-authored SeedPlanManifest at the no-head boundary is seeded as
// revision 1 (author=caller) and the run suspends at waiting_approval — no
// planner ever spawns (PATH is masked, so any CLI spawn attempt would fail
// the run instead of suspending it).
func TestReloadPlanLayers_SeedPlanManifest_SkipsPlanner(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // a planner spawn attempt would markFailed, not suspend

	env := newTestEnv(t)
	addFanoutTemplate(t, env, "test", "fanout-tmpl-seed")
	env.createTicket(t, "PB-SEED", "caller-seeded plan")
	wfiID := env.initWorkflow(t, "PB-SEED")

	manifest, err := json.Marshal(validManifest("caller goal", "fanout-tmpl-seed"))
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}

	svcWf, workflows, agents := buildPlanReloadInputs(t, env, "test")
	layerGroups := groupPhasesByLayer(svcWf.Phases)
	req := RunRequest{ProjectID: env.project, TicketID: "PB-SEED", WorkflowName: "test", ScopeType: "ticket", SeedPlanManifest: manifest}

	_, extended, terminal, worktreeHandled := env.orch.reloadPlanLayers(
		context.Background(), wfiID, req, env.pool, svcWf, layerGroups, map[int]string{}, workflows, agents)

	if extended || !terminal || !worktreeHandled {
		t.Fatalf("got (extended=%v, terminal=%v, worktreeHandled=%v), want (false, true, true) — suspend, not fail", extended, terminal, worktreeHandled)
	}
	wi := env.getWorkflowInstance(t, wfiID)
	if wi.Status != model.WorkflowInstanceWaitingApproval {
		t.Errorf("status = %v, want waiting_approval", wi.Status)
	}

	revs, err := repo.NewPlanRepo(env.pool, clock.Real()).ListRevisions(wfiID)
	if err != nil {
		t.Fatalf("ListRevisions: %v", err)
	}
	if len(revs) != 1 {
		t.Fatalf("len(revisions) = %d, want 1 (the seeded manifest only)", len(revs))
	}
	if revs[0].Author != model.PlanAuthorCaller {
		t.Errorf("author = %q, want %q", revs[0].Author, model.PlanAuthorCaller)
	}
	var m service.PlanManifest
	if err := json.Unmarshal([]byte(revs[0].Manifest), &m); err != nil || m.Goal != "caller goal" {
		t.Errorf("seeded manifest goal = %q (err=%v), want %q", m.Goal, err, "caller goal")
	}
}

// An invalid seeded manifest fails the run with the validation error rather
// than falling back to the planner.
func TestReloadPlanLayers_SeedPlanManifest_InvalidFailsRun(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	env := newTestEnv(t)
	addFanoutTemplate(t, env, "test", "fanout-tmpl-badseed")
	env.createTicket(t, "PB-BADSEED", "invalid caller plan")
	wfiID := env.initWorkflow(t, "PB-BADSEED")

	svcWf, workflows, agents := buildPlanReloadInputs(t, env, "test")
	layerGroups := groupPhasesByLayer(svcWf.Phases)
	req := RunRequest{ProjectID: env.project, TicketID: "PB-BADSEED", WorkflowName: "test", ScopeType: "ticket", SeedPlanManifest: json.RawMessage(`{"not":"a plan"}`)}

	_, extended, terminal, worktreeHandled := env.orch.reloadPlanLayers(
		context.Background(), wfiID, req, env.pool, svcWf, layerGroups, map[int]string{}, workflows, agents)

	if extended || !terminal || worktreeHandled {
		t.Fatalf("got (extended=%v, terminal=%v, worktreeHandled=%v), want (false, true, false)", extended, terminal, worktreeHandled)
	}
	if wi := env.getWorkflowInstance(t, wfiID); wi.Status != model.WorkflowInstanceFailed {
		t.Errorf("status = %v, want failed (invalid seed manifest)", wi.Status)
	}
}
