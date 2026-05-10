package orchestrator

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/service"
	"be/internal/types"
)

// setNextWorkflowOnSuccess sets next_workflow_on_success on a workflow via the service layer.
func (e *testEnv) setNextWorkflowOnSuccess(t *testing.T, workflowID, nextWorkflow string) {
	t.Helper()
	wfSvc := service.NewWorkflowService(e.pool, clock.Real())
	if err := wfSvc.UpdateWorkflowDef(e.project, workflowID, &types.WorkflowDefUpdateRequest{
		NextWorkflowOnSuccess: &nextWorkflow,
	}); err != nil {
		t.Fatalf("setNextWorkflowOnSuccess: %v", err)
	}
}

// createNextWorkflow creates a project-scoped "next-wf" workflow and wires it
// as the next_workflow_on_success for the "test" source workflow.
func (e *testEnv) createNextWorkflow(t *testing.T) {
	t.Helper()
	e.createWorkflowWithAgents(t, "next-wf", "next workflow", "project", []struct {
		ID    string
		Layer int
	}{
		{"next-agent", 0},
	})
	e.setNextWorkflowOnSuccess(t, "test", "next-wf")
}

// TestMaybeStartNextOnSuccess_HappyPath verifies that when next_workflow_on_success is set
// and finalResult is non-empty, a new project-scoped workflow_instances row is created for
// the target workflow with the correct scope_type and user_instructions.
func TestMaybeStartNextOnSuccess_HappyPath(t *testing.T) {
	env := newTestEnv(t)
	env.createNextWorkflow(t)

	req := RunRequest{
		ProjectID:    env.project,
		WorkflowName: "test",
		ChainDepth:   0,
	}
	env.orch.maybeStartNextOnSuccess(context.Background(), req, "the summary")

	if !pollUntil(2*time.Second, func() bool {
		return env.countProjectInstances(t, "next-wf") >= 1
	}) {
		t.Fatal("timeout: next-wf workflow_instances row was not created")
	}

	var scopeType, findingsJSON string
	if err := env.pool.QueryRow(
		`SELECT scope_type, COALESCE(findings, '{}') FROM workflow_instances
		 WHERE LOWER(project_id) = LOWER(?) AND LOWER(workflow_id) = LOWER('next-wf')`,
		env.project,
	).Scan(&scopeType, &findingsJSON); err != nil {
		t.Fatalf("query new instance: %v", err)
	}
	if scopeType != "project" {
		t.Errorf("scope_type = %q, want 'project'", scopeType)
	}
	var findings map[string]interface{}
	if err := json.Unmarshal([]byte(findingsJSON), &findings); err != nil {
		t.Fatalf("unmarshal findings: %v", err)
	}
	if findings["user_instructions"] != "the summary" {
		t.Errorf("user_instructions = %v, want 'the summary'", findings["user_instructions"])
	}
}

// TestMaybeStartNextOnSuccess_EmptySummary_NoSpawn verifies that when finalResult is empty,
// no new instance is created.
func TestMaybeStartNextOnSuccess_EmptySummary_NoSpawn(t *testing.T) {
	env := newTestEnv(t)
	env.createNextWorkflow(t)

	req := RunRequest{ProjectID: env.project, WorkflowName: "test"}
	env.orch.maybeStartNextOnSuccess(context.Background(), req, "")

	if pollUntil(200*time.Millisecond, func() bool {
		return env.countProjectInstances(t, "next-wf") > 0
	}) {
		t.Fatal("instance created despite empty finalResult")
	}
}

// TestMaybeStartNextOnSuccess_DepthCap_NoSpawn verifies that when ChainDepth has reached
// maxNextWorkflowOnSuccessDepth (10), no new instance is created.
func TestMaybeStartNextOnSuccess_DepthCap_NoSpawn(t *testing.T) {
	env := newTestEnv(t)
	env.createNextWorkflow(t)

	req := RunRequest{
		ProjectID:    env.project,
		WorkflowName: "test",
		ChainDepth:   maxNextWorkflowOnSuccessDepth,
	}
	env.orch.maybeStartNextOnSuccess(context.Background(), req, "summary")

	if pollUntil(200*time.Millisecond, func() bool {
		return env.countProjectInstances(t, "next-wf") > 0
	}) {
		t.Fatal("instance created despite depth cap (ChainDepth=10)")
	}
}

// TestMaybeStartNextOnSuccess_CancelledCtx_NoSpawn verifies that when ctx is already
// cancelled before calling, no new instance is created.
func TestMaybeStartNextOnSuccess_CancelledCtx_NoSpawn(t *testing.T) {
	env := newTestEnv(t)
	env.createNextWorkflow(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	req := RunRequest{ProjectID: env.project, WorkflowName: "test"}
	env.orch.maybeStartNextOnSuccess(ctx, req, "summary")

	if pollUntil(200*time.Millisecond, func() bool {
		return env.countProjectInstances(t, "next-wf") > 0
	}) {
		t.Fatal("instance created despite cancelled context")
	}
}

// TestMaybeStartNextOnSuccess_NoNextWorkflowConfigured_NoSpawn verifies that when the
// source workflow has no next_workflow_on_success set, no new instance is created.
func TestMaybeStartNextOnSuccess_NoNextWorkflowConfigured_NoSpawn(t *testing.T) {
	env := newTestEnv(t)
	// "test" has next_workflow_on_success="" by default; no additional setup needed.

	req := RunRequest{ProjectID: env.project, WorkflowName: "test"}
	env.orch.maybeStartNextOnSuccess(context.Background(), req, "summary")

	if pollUntil(200*time.Millisecond, func() bool {
		return env.countProjectInstances(t, "test") > 0
	}) {
		t.Fatal("unexpected instance created when next_workflow_on_success is empty")
	}
}
