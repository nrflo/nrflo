package console

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"be/internal/model"
	"be/internal/repo"
	"be/internal/service"
)

// seedPlanBoundaryInstance seeds a project-scoped workflow_instance at status
// 'planning' with a fanout_template agent_definition and one draft plan
// revision referencing it — the minimum state approve_plan needs to reach
// PlanService.Approve's success path (mirrors orchestrator's
// addFanoutTemplate/validManifest/appendDraftPlan, plan_boundary_test.go:19-70).
// Returns the seeded revision number.
func (e *consoleTestEnv) seedPlanBoundaryInstance(t *testing.T, projectID, instanceID, templateID string) int {
	t.Helper()
	now := e.clk.Now().UTC().Format(time.RFC3339Nano)
	wfName := "wf-" + instanceID
	mustExec(t, e.pool, `INSERT INTO workflows (id, project_id, description, created_at, updated_at, scope_type, groups)
		VALUES (?, ?, '', ?, ?, 'project', '[]')`, wfName, projectID, now, now)
	mustExec(t, e.pool, `INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, scope_type, retry_count, created_at, updated_at)
		VALUES (?, ?, '', ?, 'planning', 'project', 0, ?, ?)`, instanceID, projectID, wfName, now, now)
	mustExec(t, e.pool, `INSERT INTO agent_definitions (id, project_id, workflow_id, node_role, model, created_at, updated_at)
		VALUES (?, ?, ?, 'fanout_template', 'sonnet-5', ?, ?)`, templateID, projectID, wfName, now, now)

	m := service.PlanManifest{
		Version: 1,
		Goal:    "do the thing",
		Layers: []service.PlanLayer{
			{Layer: 0, Policy: "all", Nodes: []service.PlanNode{
				{ID: "step1", Template: templateID, Instructions: "do the thing"},
			}},
		},
	}
	canonical, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	rev, err := repo.NewPlanRepo(e.pool, e.clk).Append(
		instanceID, string(canonical), service.HashManifest(m), model.PlanAuthorCaller, "", m.Goal)
	if err != nil {
		t.Fatalf("Append: %v", err)
	}
	return rev
}

// TestApprovePlan_ClaimedBoundary_ReturnsNoteWithoutResuming: a live plan
// boundary claims the approval — the handler must not call
// ResumeAfterPlanApproval at all, and the response carries the approved
// revision plus a "note" field.
func TestApprovePlan_ClaimedBoundary_ReturnsNoteWithoutResuming(t *testing.T) {
	env := newConsoleTestEnv(t)
	fake := &fakeOrchestrator{claimResult: true}
	env.deps.Orch = fake
	rev := env.seedPlanBoundaryInstance(t, testProjectID, "wfi-approve-claimed", "tmpl-claimed")
	reg, err := BuildRegistry(env.deps, nil)
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}
	toolEnv := NewToolEnv(env.deps, "sess-1", testProjectID, model.AgentSessionKindConsole)

	out, isErr, err := invoke(t, reg, toolEnv, "approve_plan",
		fmt.Sprintf(`{"instance_id":"wfi-approve-claimed","revision":%d}`, rev))
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if isErr {
		t.Fatalf("isErr = true, want false; out=%s", out)
	}
	if !strings.Contains(out, `"note"`) {
		t.Errorf("out = %q, want a note field for a claimed boundary", out)
	}
	if fake.resumeInstanceID != "" {
		t.Errorf("resumeInstanceID = %q, want empty — ResumeAfterPlanApproval must not be called when the boundary claims", fake.resumeInstanceID)
	}
	if fake.claimInstanceID != "wfi-approve-claimed" {
		t.Errorf("claimInstanceID = %q, want wfi-approve-claimed", fake.claimInstanceID)
	}
}

// TestApprovePlan_UnclaimedBoundary_ResumeFails_ReturnsApprovedButResumeFailed:
// no live boundary claims the approval, so the handler falls back to
// ResumeAfterPlanApproval; when that fails, the plan IS approved but the
// legacy "approved but resume failed" text (and isErr=true) must still be
// returned so a caller knows to retry the same revision.
func TestApprovePlan_UnclaimedBoundary_ResumeFails_ReturnsApprovedButResumeFailed(t *testing.T) {
	env := newConsoleTestEnv(t)
	fake := &fakeOrchestrator{claimResult: false, resumeErr: errors.New("resume boom")}
	env.deps.Orch = fake
	rev := env.seedPlanBoundaryInstance(t, testProjectID, "wfi-approve-parked", "tmpl-parked")
	reg, err := BuildRegistry(env.deps, nil)
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}
	toolEnv := NewToolEnv(env.deps, "sess-1", testProjectID, model.AgentSessionKindConsole)

	out, isErr, err := invoke(t, reg, toolEnv, "approve_plan",
		fmt.Sprintf(`{"instance_id":"wfi-approve-parked","revision":%d}`, rev))
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if !isErr || !strings.Contains(out, "approved but resume failed") {
		t.Errorf("out=%q isErr=%v, want isErr=true and 'approved but resume failed'", out, isErr)
	}
	if fake.resumeInstanceID != "wfi-approve-parked" {
		t.Errorf("resumeInstanceID = %q, want wfi-approve-parked", fake.resumeInstanceID)
	}
}
