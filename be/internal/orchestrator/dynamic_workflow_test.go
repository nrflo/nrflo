package orchestrator

import (
	"context"
	"strings"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/service"
	"be/internal/types"
	"be/internal/ws"
)

// TestStartDynamicWorkflow_ModeAuto_RefusedWhenGateOff: mode="auto" must be
// rejected before ever touching startChildRun (no charge, no o.Start) when
// service.DynamicAutoEnabled is false for the project (the default).
func TestStartDynamicWorkflow_ModeAuto_RefusedWhenGateOff(t *testing.T) {
	env := newTestEnv(t)
	parentID := env.initProjectWorkflow(t, "test")

	_, err := env.orch.StartDynamicWorkflow(context.Background(), parentID, env.project, "do the thing", "auto")
	if err == nil || !strings.Contains(err.Error(), "disabled") {
		t.Fatalf("want gate-disabled error, got %v", err)
	}

	// No budget should have been charged since the gate check runs before startChildRun.
	var starts int
	if err := env.pool.QueryRow(`SELECT subworkflow_starts FROM workflow_instances WHERE id=?`, parentID).Scan(&starts); err != nil {
		t.Fatal(err)
	}
	if starts != 0 {
		t.Errorf("subworkflow_starts = %d, want 0 (gate must refuse before charging budget)", starts)
	}
}

// TestStartDynamicWorkflow_ModeAuto_GateOnPassesThroughToSharedGuards: with
// the gate enabled, mode="auto" must reach startChildRun (shared with
// StartSubworkflow) rather than being refused for the mode itself — proven
// here by the fact that it fails on the *next* guard (the bundled `dynamic`
// workflow is not seeded in this test project) instead of the gate error.
func TestStartDynamicWorkflow_ModeAuto_GateOnPassesThroughToSharedGuards(t *testing.T) {
	env := newTestEnv(t)
	parentID := env.initProjectWorkflow(t, "test")

	if err := env.pool.SetProjectConfig(env.project, service.DynamicWorkflowAutoEnabledKey, "true"); err != nil {
		t.Fatalf("SetProjectConfig: %v", err)
	}

	_, err := env.orch.StartDynamicWorkflow(context.Background(), parentID, env.project, "do the thing", "auto")
	if err == nil {
		t.Fatal("want an error: the bundled `dynamic` workflow is not seeded in this test project")
	}
	if strings.Contains(err.Error(), "disabled") {
		t.Errorf("got the gate-disabled error even though the gate is on: %v", err)
	}
}

// TestStartDynamicWorkflow_UnknownMode_Rejected covers the mode validation
// switch's default case.
func TestStartDynamicWorkflow_UnknownMode_Rejected(t *testing.T) {
	env := newTestEnv(t)
	parentID := env.initProjectWorkflow(t, "test")

	_, err := env.orch.StartDynamicWorkflow(context.Background(), parentID, env.project, "do the thing", "bogus")
	if err == nil || !strings.Contains(err.Error(), "unknown mode") {
		t.Fatalf("want unknown-mode error, got %v", err)
	}
}

// TestStartDynamicWorkflow_ModeApprove_SharesGuardsWithStartSubworkflow: the
// default mode ("") reaches the same startChildRun guard chain as
// StartSubworkflow — proven by the same "workflow not seeded" failure as the
// auto-mode case above, with no gate check involved at all.
func TestStartDynamicWorkflow_ModeApprove_SharesGuardsWithStartSubworkflow(t *testing.T) {
	env := newTestEnv(t)
	parentID := env.initProjectWorkflow(t, "test")

	_, err := env.orch.StartDynamicWorkflow(context.Background(), parentID, env.project, "do the thing", "")
	if err == nil {
		t.Fatal("want an error: the bundled `dynamic` workflow is not seeded in this test project")
	}
}

// TestRevisePlan_OwnershipEnforced mirrors
// TestGetSubworkflow_ParentageAuthorization for the RevisePlan tool path:
// only the run that started a child may drive its plan.
func TestRevisePlan_OwnershipEnforced(t *testing.T) {
	env := newTestEnv(t)
	wfiID := env.initProjectWorkflow(t, "test")
	seedChildInstance(t, env, wfiID, "parent-1", "waiting_approval", "")

	req := types.PlanReviseRequest{Revision: 0, Manifest: []byte(validPlanManifestRaw("fanout-tmpl"))}

	if _, err := env.orch.RevisePlan(context.Background(), "other-caller", env.project, wfiID, req); err == nil {
		t.Error("want error for a caller that did not start the child")
	}
	if _, err := env.orch.RevisePlan(context.Background(), "parent-1", "other-project", wfiID, req); err == nil {
		t.Error("want error for foreign project")
	}
}

// TestApprovePlan_OwnershipEnforced mirrors TestRevisePlan_OwnershipEnforced
// for the ApprovePlan tool path.
func TestApprovePlan_OwnershipEnforced(t *testing.T) {
	env := newTestEnv(t)
	wfiID := env.initProjectWorkflow(t, "test")
	seedChildInstance(t, env, wfiID, "parent-1", "waiting_approval", "")

	if _, err := env.orch.ApprovePlan(context.Background(), "other-caller", env.project, wfiID, 1); err == nil {
		t.Error("want error for a caller that did not start the child")
	}
	if _, err := env.orch.ApprovePlan(context.Background(), "parent-1", "other-project", wfiID, 1); err == nil {
		t.Error("want error for foreign project")
	}
}

// TestRevisePlan_HappyPath_AppendsRevisionAndBroadcasts drives a child's plan
// lifecycle from the parent's side via a caller-supplied manifest (never
// touches the planner — see plan_boundary_test.go's
// TestReloadPlanLayers_NoPlanHead_DraftAttemptFailsWithoutPlannerCLI for why
// that path is deliberately not exercised through a live run here).
func TestRevisePlan_HappyPath_AppendsRevisionAndBroadcasts(t *testing.T) {
	env := newTestEnv(t)
	addFanoutTemplate(t, env, "test", "fanout-tmpl")
	childID := env.initProjectWorkflow(t, "test")
	seedChildInstance(t, env, childID, "parent-1", "waiting_approval", "")
	ch := env.subscribeWSClient(t, "ws-revise", "")

	req := types.PlanReviseRequest{Revision: 0, Manifest: []byte(validPlanManifestRaw("fanout-tmpl"))}
	rev, err := env.orch.RevisePlan(context.Background(), "parent-1", env.project, childID, req)
	if err != nil {
		t.Fatalf("RevisePlan: %v", err)
	}
	if rev.Revision != 1 {
		t.Errorf("rev.Revision = %d, want 1", rev.Revision)
	}
	if rev.Author != model.PlanAuthorCaller {
		t.Errorf("rev.Author = %q, want %q", rev.Author, model.PlanAuthorCaller)
	}

	event := expectEvent(t, ch, ws.EventPlanDrafted, 2*time.Second)
	if event.Data["instance_id"] != childID {
		t.Errorf("event instance_id = %v, want %v", event.Data["instance_id"], childID)
	}

	// A second revise at the same (now stale) revision must be rejected.
	if _, err := env.orch.RevisePlan(context.Background(), "parent-1", env.project, childID, req); err == nil {
		t.Error("want stale-revision error on a repeat revise at revision 0")
	}
}

// TestApprovePlan_HappyPath_MaterializesAndResumes: approving a plan-suspended
// child materializes it and resumes the run (mirrors
// TestResumeAfterPlanApproval_HappyPath_RelaunchesAtMinMaterializedLayer's
// safety note — the relaunched runLoop will attempt to spawn a real agent for
// the materialized node; stop the run immediately after asserting the
// resume/broadcast mechanics, never waiting for it to actually execute).
func TestApprovePlan_HappyPath_MaterializesAndResumes(t *testing.T) {
	env := newTestEnv(t)
	addFanoutTemplate(t, env, "test", "fanout-tmpl")
	childID := env.initProjectWorkflow(t, "test")
	seedChildInstance(t, env, childID, "parent-1", "active", "")

	rev := appendDraftPlan(t, env, childID, validManifest("do the thing", "fanout-tmpl"))
	if err := repo.NewWorkflowInstanceRepo(env.pool, clock.Real()).UpdateStatus(childID, model.WorkflowInstanceWaitingApproval); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	ch := env.subscribeWSClient(t, "ws-approve", "")

	approved, err := env.orch.ApprovePlan(context.Background(), "parent-1", env.project, childID, rev)
	if err != nil {
		t.Fatalf("ApprovePlan: %v", err)
	}
	if approved.Revision != rev {
		t.Errorf("approved.Revision = %d, want %d", approved.Revision, rev)
	}

	expectEvent(t, ch, ws.EventPlanApproved, 2*time.Second)
	expectEvent(t, ch, ws.EventPlanMaterialized, 2*time.Second)
	expectEvent(t, ch, ws.EventWorkflowResumed, 2*time.Second)

	env.orch.mu.Lock()
	_, running := env.orch.runs[childID]
	env.orch.mu.Unlock()
	if !running {
		t.Errorf("want a runState registered for %s after ApprovePlan resumed it", childID)
	}

	env.stopAndWaitRun(t, childID)
}

// TestApprovePlan_StaleRevision_Rejected: a revision that doesn't match the
// child's current head is rejected without materializing anything.
func TestApprovePlan_StaleRevision_Rejected(t *testing.T) {
	env := newTestEnv(t)
	addFanoutTemplate(t, env, "test", "fanout-tmpl")
	childID := env.initProjectWorkflow(t, "test")
	seedChildInstance(t, env, childID, "parent-1", "waiting_approval", "")

	appendDraftPlan(t, env, childID, validManifest("do the thing", "fanout-tmpl"))

	if _, err := env.orch.ApprovePlan(context.Background(), "parent-1", env.project, childID, 99); err == nil {
		t.Error("want stale-revision error for a revision that doesn't match the head")
	}
}

// validPlanManifestRaw builds a minimal, valid v1 plan manifest JSON string
// referencing templateID, mirroring plan_boundary_test.go's validManifest but
// as a raw JSON string (types.PlanReviseRequest.Manifest is json.RawMessage).
func validPlanManifestRaw(templateID string) string {
	return `{"version":1,"goal":"do the thing","layers":[{"layer":0,"policy":"any","nodes":[{"id":"n1","template":"` + templateID + `","instructions":"do it"}]}]}`
}
