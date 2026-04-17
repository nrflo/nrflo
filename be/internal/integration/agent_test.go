package integration

import (
	"testing"

	"be/internal/types"
)

func TestAgentFail(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "AGT-1", "Agent fail")
	env.InitWorkflow(t, "AGT-1")

	wfiID := env.GetWorkflowInstanceID(t, "AGT-1", "test")

	// Create running agent session via DB
	env.InsertAgentSession(t, "sess-2", "AGT-1", wfiID, "analyzer", "builder", "opus_4_7")

	// Fail builder via socket — context derived from session on the server
	env.MustExecute(t, "agent.fail", map[string]interface{}{
		"session_id":  "sess-2",
		"instance_id": wfiID,
	}, nil)

	// Verify builder session has result "fail" via service
	session, err := env.AgentSvc.GetSessionByID("sess-2")
	if err != nil {
		t.Fatalf("failed to get session: %v", err)
	}
	if session.Result.String != "fail" {
		t.Fatalf("expected builder result 'fail', got %v", session.Result.String)
	}
}

func TestAgentContinue(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "AGT-2", "Agent continue")
	env.InitWorkflow(t, "AGT-2")

	wfiID := env.GetWorkflowInstanceID(t, "AGT-2", "test")
	env.InsertAgentSession(t, "sess-cont-1", "AGT-2", wfiID, "analyzer", "analyzer", "sonnet")

	// Continue analyzer via socket
	env.MustExecute(t, "agent.continue", map[string]interface{}{
		"session_id":  "sess-cont-1",
		"instance_id": wfiID,
	}, nil)

	// Verify session result is "continue" via service
	session, err := env.AgentSvc.GetSessionByID("sess-cont-1")
	if err != nil {
		t.Fatalf("failed to get session: %v", err)
	}
	if session.Result.String != "continue" {
		t.Fatalf("expected analyzer result 'continue', got %v", session.Result.String)
	}
}

func TestAgentGetActive(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "AGT-3", "Active agents")
	env.InitWorkflow(t, "AGT-3")

	wfiID := env.GetWorkflowInstanceID(t, "AGT-3", "test")
	env.InsertAgentSession(t, "sess-active", "AGT-3", wfiID, "analyzer", "analyzer", "sonnet")

	// Get active via service (socket no longer supports agent.active)
	result, err := env.AgentSvc.GetActive(env.ProjectID, "AGT-3", &types.AgentActiveRequest{
		Workflow: "test",
	})
	if err != nil {
		t.Fatalf("failed to get active agents: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 active agent, got %d", len(result))
	}
	if result[0].AgentType != "analyzer" {
		t.Fatalf("expected agent_type 'analyzer', got %v", result[0].AgentType)
	}
	if result[0].SessionID != "sess-active" {
		t.Fatalf("expected session_id 'sess-active', got %v", result[0].SessionID)
	}
}

func TestAgentSessions(t *testing.T) {
	env := NewTestEnv(t)

	// Create tickets and init workflows
	env.CreateTicket(t, "ticket-a", "Ticket A")
	env.InitWorkflow(t, "ticket-a")
	wfiA := env.GetWorkflowInstanceID(t, "ticket-a", "test")

	env.CreateTicket(t, "ticket-b", "Ticket B")
	env.InitWorkflow(t, "ticket-b")
	wfiB := env.GetWorkflowInstanceID(t, "ticket-b", "test")

	env.InsertAgentSession(t, "sess-1", "ticket-a", wfiA, "analyzer", "analyzer", "sonnet")
	env.InsertAgentSession(t, "sess-2", "ticket-a", wfiA, "builder", "builder", "sonnet")
	env.InsertAgentSession(t, "sess-3", "ticket-b", wfiB, "analyzer", "analyzer", "sonnet")

	// Get all sessions via service
	allSessions, err := env.AgentSvc.GetRecentSessions(env.ProjectID, 20)
	if err != nil {
		t.Fatalf("failed to get sessions: %v", err)
	}
	if len(allSessions) != 3 {
		t.Fatalf("expected 3 sessions, got %d", len(allSessions))
	}

	// Get sessions for specific ticket via service
	ticketASessions, err := env.AgentSvc.GetTicketSessions(env.ProjectID, "ticket-a", "")
	if err != nil {
		t.Fatalf("failed to get ticket-a sessions: %v", err)
	}
	if len(ticketASessions) != 2 {
		t.Fatalf("expected 2 sessions for ticket-a, got %d", len(ticketASessions))
	}

	// Get sessions for ticket-b
	ticketBSessions, err := env.AgentSvc.GetTicketSessions(env.ProjectID, "ticket-b", "")
	if err != nil {
		t.Fatalf("failed to get ticket-b sessions: %v", err)
	}
	if len(ticketBSessions) != 1 {
		t.Fatalf("expected 1 session for ticket-b, got %d", len(ticketBSessions))
	}
}
