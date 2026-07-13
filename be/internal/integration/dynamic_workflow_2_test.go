package integration

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"be/internal/model"
	"be/internal/orchestrator"
	"be/internal/repo"
	"be/internal/service"
	"be/internal/types"
	"be/internal/ws"
)

// TestDynamicWorkflow_ApprovePlan_ResumesPlanSuspendedChild proves
// ApprovePlan's resume-trigger fires: when the child instance is parked
// plan-suspended (waiting_approval) at the time of approval, ApprovePlan must
// call ResumeAfterPlanApproval and relaunch the run's runLoop. This mirrors
// orchestrator's own
// TestResumeAfterPlanApproval_HappyPath_RelaunchesAtMinMaterializedLayer
// (be/internal/orchestrator/plan_resume_test.go) as closely as possible from
// this package, including its immediate-stop safety pattern: the relaunched
// runLoop is about to spawn a real agent for the materialized fanout_template
// node, so this test asserts only the resume/relaunch signal (the
// EventWorkflowResumed broadcast) and then calls orch.Stop immediately --
// this package cannot reach into the orchestrator package's unexported
// o.runs/o.mu to assert runState registration directly (different package),
// so orch.Stop's own fallback (forceStopInstance when nothing is registered)
// makes the immediate call safe either way.
func TestDynamicWorkflow_ApprovePlan_ResumesPlanSuspendedChild(t *testing.T) {
	env := NewTestEnv(t)

	// "test" (seeded by NewTestEnv) already has analyzer(L0)/builder(L1)
	// static defs; add a fanout_template so a plan can materialize against it.
	if _, err := env.getAgentDefService(t).CreateAgentDef(env.ProjectID, "test", &types.AgentDefCreateRequest{
		ID:            "fanout-tmpl",
		Prompt:        "do work",
		Layer:         0,
		NodeRole:      "fanout_template",
		Description:   "fanout template",
		Model:         "sonnet",
		ExecutionMode: "cli_interactive",
	}); err != nil {
		t.Fatalf("create fanout_template def: %v", err)
	}

	const ticketID = "DYNWF-RESUME-1"
	env.CreateTicket(t, ticketID, "resume via approve_plan")
	wi, err := env.WorkflowSvc.Init(env.ProjectID, ticketID, &types.WorkflowInitRequest{Workflow: "test"})
	if err != nil {
		t.Fatalf("init workflow: %v", err)
	}
	wfiID := wi.ID

	const callerID = "caller-dynwf-resume-1"
	if _, err := env.Pool.Exec(`UPDATE workflow_instances SET parent_instance_id = ? WHERE id = ?`, callerID, wfiID); err != nil {
		t.Fatalf("set parent_instance_id: %v", err)
	}
	if err := repo.NewWorkflowInstanceRepo(env.Pool, env.Clock).UpdateStatus(wfiID, model.WorkflowInstanceWaitingApproval); err != nil {
		t.Fatalf("UpdateStatus(waiting_approval): %v", err)
	}

	orch := orchestrator.New(env.Pool.Path, env.Hub, env.Clock, nil, "")
	ctx := context.Background()
	_, ch := env.NewWSClient(t, "ws-dynwf-resume-1", ticketID)

	manifest := buildSingleNodeManifest("resume via approve_plan", "fanout-tmpl")
	raw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	rev, err := orch.RevisePlan(ctx, callerID, env.ProjectID, wfiID, types.PlanReviseRequest{Revision: 0, Manifest: raw})
	if err != nil {
		t.Fatalf("RevisePlan: %v", err)
	}
	drainChannel(ch) // discard plan.drafted -- this test only cares about the resume signal

	if _, err := orch.ApprovePlan(ctx, callerID, env.ProjectID, wfiID, rev.Revision); err != nil {
		t.Fatalf("ApprovePlan: %v", err)
	}

	// ApprovePlan must have driven ResumeAfterPlanApproval: the plan-suspended
	// instance flips back to active and its runLoop relaunches at the first
	// materialized layer, broadcasting EventWorkflowResumed.
	expectEvent(t, ch, ws.EventWorkflowResumed, 2*time.Second)

	// Stop immediately, before the relaunched runLoop reaches a real agent
	// spawn -- see the safety note in the doc comment above.
	if err := orch.Stop(wfiID); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}

// TestDynamicWorkflow_AutoEnabled_DefaultFalse: mode="auto" on
// StartDynamicWorkflow is refused unless dynamic_workflow_auto_enabled is
// explicitly turned on for the project; assert the project-scoped default
// here (the mode="auto" gate itself is covered in the orchestrator package).
func TestDynamicWorkflow_AutoEnabled_DefaultFalse(t *testing.T) {
	env := NewTestEnv(t)
	if service.DynamicAutoEnabled(env.Pool, env.ProjectID) {
		t.Error("DynamicAutoEnabled default = true, want false")
	}
}
