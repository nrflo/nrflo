package orchestrator

import (
	"context"
	"testing"

	"be/internal/model"
)

// TestOrchestrator_Start_ProjectScope_OriginHuman_PersistsEmptySessionID verifies
// that a project-scoped Start with Origin=model.RunOriginHuman (mirroring the
// HTTP dynamic-workflow start path) persists origin="human" and an empty
// origin_session_id on the created instance.
func TestOrchestrator_Start_ProjectScope_OriginHuman_PersistsEmptySessionID(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)

	env.createWorkflowWithAgents(t, "wf-origin-human", "Origin human workflow", "project", []struct {
		ID    string
		Layer int
	}{
		{ID: "agent-1", Layer: 0},
	})

	result, err := env.orch.Start(context.Background(), RunRequest{
		ProjectID:    env.project,
		WorkflowName: "wf-origin-human",
		ScopeType:    "project",
		Origin:       model.RunOriginHuman,
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer env.stopAndWaitRun(t, result.InstanceID)

	wi := env.getWorkflowInstance(t, result.InstanceID)
	if wi.Origin != model.RunOriginHuman {
		t.Errorf("Origin = %q, want %q", wi.Origin, model.RunOriginHuman)
	}
	if wi.OriginSessionID != "" {
		t.Errorf("OriginSessionID = %q, want empty", wi.OriginSessionID)
	}
}

// TestOrchestrator_StartConsoleWorkflow_PersistsConsoleOriginAndSessionID verifies
// that StartConsoleWorkflow (the console-launched entrypoint) attributes the
// created instance's origin/origin_session_id to console + the launching
// console session id, for both project- and ticket-scoped runs.
func TestOrchestrator_StartConsoleWorkflow_PersistsConsoleOriginAndSessionID(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)

	env.createWorkflowWithAgents(t, "wf-origin-console", "Origin console workflow", "project", []struct {
		ID    string
		Layer int
	}{
		{ID: "agent-1", Layer: 0},
	})

	instanceID, err := env.orch.StartConsoleWorkflow(context.Background(), env.project, "", "wf-origin-console", "do it", "project", "sess-console-xyz")
	if err != nil {
		t.Fatalf("StartConsoleWorkflow: %v", err)
	}
	defer env.stopAndWaitRun(t, instanceID)

	wi := env.getWorkflowInstance(t, instanceID)
	if wi.Origin != model.RunOriginConsole {
		t.Errorf("Origin = %q, want %q", wi.Origin, model.RunOriginConsole)
	}
	if wi.OriginSessionID != "sess-console-xyz" {
		t.Errorf("OriginSessionID = %q, want %q", wi.OriginSessionID, "sess-console-xyz")
	}
}

// TestOrchestrator_StartConsoleWorkflow_TicketScope_PersistsConsoleOrigin verifies
// the ticket-scoped console entrypoint (workflow_run) attributes origin the same way.
func TestOrchestrator_StartConsoleWorkflow_TicketScope_PersistsConsoleOrigin(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	env.createTicket(t, "tkt-origin-console", "Origin console ticket")

	instanceID, err := env.orch.StartConsoleWorkflow(context.Background(), env.project, "tkt-origin-console", "test", "do it", "ticket", "sess-console-ticket")
	if err != nil {
		t.Fatalf("StartConsoleWorkflow: %v", err)
	}
	defer env.stopAndWaitRun(t, instanceID)

	wi := env.getWorkflowInstance(t, instanceID)
	if wi.Origin != model.RunOriginConsole {
		t.Errorf("Origin = %q, want %q", wi.Origin, model.RunOriginConsole)
	}
	if wi.OriginSessionID != "sess-console-ticket" {
		t.Errorf("OriginSessionID = %q, want %q", wi.OriginSessionID, "sess-console-ticket")
	}
}
