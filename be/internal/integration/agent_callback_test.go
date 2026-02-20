package integration

import (
	"encoding/json"
	"testing"

	"be/internal/model"
	"be/internal/types"
)

func TestAgentCallback(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "AGT-CB-1", "Agent callback")
	env.InitWorkflow(t, "AGT-CB-1")

	wfiID := env.GetWorkflowInstanceID(t, "AGT-CB-1", "test")
	env.InsertAgentSession(t, "sess-cb-1", "AGT-CB-1", wfiID, "analyzer", "analyzer", "sonnet")

	// Call agent.callback via socket
	env.MustExecute(t, "agent.callback", map[string]interface{}{
		"ticket_id":   "AGT-CB-1",
		"workflow":    "test",
		"agent_type":  "analyzer",
		"session_id":  "sess-cb-1",
		"level":       1,
		"instance_id": wfiID,
	}, nil)

	// Verify session result is "callback" via service
	session, err := env.AgentSvc.GetSessionByID("sess-cb-1")
	if err != nil {
		t.Fatalf("failed to get session: %v", err)
	}
	if session.Result.String != "callback" {
		t.Fatalf("expected analyzer result 'callback', got %v", session.Result.String)
	}

	// Verify callback_level finding is saved
	findings := session.GetFindings()
	level, ok := findings["callback_level"]
	if !ok {
		t.Fatalf("expected callback_level finding to be set")
	}
	// JSON unmarshaling converts numbers to float64
	levelFloat, ok := level.(float64)
	if !ok {
		t.Fatalf("expected callback_level to be a number, got %T", level)
	}
	if int(levelFloat) != 1 {
		t.Fatalf("expected callback_level to be 1, got %v", levelFloat)
	}
}

func TestAgentCallbackWithModel(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "AGT-CB-2", "Agent callback with model")
	env.InitWorkflow(t, "AGT-CB-2")

	wfiID := env.GetWorkflowInstanceID(t, "AGT-CB-2", "test")
	env.InsertAgentSession(t, "sess-cb-2", "AGT-CB-2", wfiID, "builder", "builder", "opus")

	// Call agent.callback with model filter
	env.MustExecute(t, "agent.callback", map[string]interface{}{
		"ticket_id":   "AGT-CB-2",
		"workflow":    "test",
		"agent_type":  "builder",
		"model":       "opus",
		"session_id":  "sess-cb-2",
		"level":       2,
		"instance_id": wfiID,
	}, nil)

	// Verify session result is "callback"
	session, err := env.AgentSvc.GetSessionByID("sess-cb-2")
	if err != nil {
		t.Fatalf("failed to get session: %v", err)
	}
	if session.Result.String != "callback" {
		t.Fatalf("expected builder result 'callback', got %v", session.Result.String)
	}

	// Verify callback_level finding
	findings := session.GetFindings()
	level, ok := findings["callback_level"]
	if !ok {
		t.Fatalf("expected callback_level finding to be set")
	}
	levelFloat := level.(float64)
	if int(levelFloat) != 2 {
		t.Fatalf("expected callback_level to be 2, got %v", levelFloat)
	}
}

func TestAgentCallbackZeroLevel(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "AGT-CB-3", "Agent callback level 0")
	env.InitWorkflow(t, "AGT-CB-3")

	wfiID := env.GetWorkflowInstanceID(t, "AGT-CB-3", "test")
	env.InsertAgentSession(t, "sess-cb-3", "AGT-CB-3", wfiID, "analyzer", "analyzer", "haiku")

	// Call agent.callback with level 0
	env.MustExecute(t, "agent.callback", map[string]interface{}{
		"ticket_id":   "AGT-CB-3",
		"workflow":    "test",
		"agent_type":  "analyzer",
		"session_id":  "sess-cb-3",
		"level":       0,
		"instance_id": wfiID,
	}, nil)

	// Verify session result
	session, err := env.AgentSvc.GetSessionByID("sess-cb-3")
	if err != nil {
		t.Fatalf("failed to get session: %v", err)
	}
	if session.Result.String != "callback" {
		t.Fatalf("expected result 'callback', got %v", session.Result.String)
	}

	// Verify callback_level is 0
	findings := session.GetFindings()
	level, ok := findings["callback_level"]
	if !ok {
		t.Fatalf("expected callback_level finding to be set")
	}
	levelFloat := level.(float64)
	if int(levelFloat) != 0 {
		t.Fatalf("expected callback_level to be 0, got %v", levelFloat)
	}
}

func TestAgentCallbackNoActiveAgent(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "AGT-CB-4", "Agent callback no active")
	env.InitWorkflow(t, "AGT-CB-4")
	wfiID := env.GetWorkflowInstanceID(t, "AGT-CB-4", "test")

	// No active agent session — expect error
	env.ExpectError(t, "agent.callback", map[string]interface{}{
		"ticket_id":   "AGT-CB-4",
		"workflow":    "test",
		"agent_type":  "analyzer",
		"session_id":  "nonexistent-session",
		"level":        1,
		"instance_id": wfiID,
	}, -32603) // Internal error
}

func TestAgentCallbackPreservesExistingFindings(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "AGT-CB-5", "Agent callback preserves findings")
	env.InitWorkflow(t, "AGT-CB-5")

	wfiID := env.GetWorkflowInstanceID(t, "AGT-CB-5", "test")
	env.InsertAgentSession(t, "sess-cb-5", "AGT-CB-5", wfiID, "analyzer", "analyzer", "sonnet")

	// Add initial findings via socket (use findings.add-bulk)
	env.MustExecute(t, "findings.add-bulk", map[string]interface{}{
		"ticket_id":  "AGT-CB-5",
		"workflow":   "test",
		"agent_type": "analyzer",
		"key_values": map[string]interface{}{
			"callback_instructions": "Fix the bug in layer 0",
			"bug_description":       "NPE in handler",
		},
		"instance_id": wfiID,
	}, nil)

	// Call agent.callback
	env.MustExecute(t, "agent.callback", map[string]interface{}{
		"ticket_id":   "AGT-CB-5",
		"workflow":    "test",
		"agent_type":  "analyzer",
		"session_id":  "sess-cb-5",
		"level":       1,
		"instance_id": wfiID,
	}, nil)

	// Verify all findings are preserved
	session, err := env.AgentSvc.GetSessionByID("sess-cb-5")
	if err != nil {
		t.Fatalf("failed to get session: %v", err)
	}

	findings := session.GetFindings()
	if findings["callback_instructions"] != "Fix the bug in layer 0" {
		t.Fatalf("expected callback_instructions to be preserved")
	}
	if findings["bug_description"] != "NPE in handler" {
		t.Fatalf("expected bug_description to be preserved")
	}
	level := findings["callback_level"].(float64)
	if int(level) != 1 {
		t.Fatalf("expected callback_level to be 1, got %v", level)
	}
}

func TestAgentCallbackStatusMapping(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "AGT-CB-6", "Agent callback status mapping")
	env.InitWorkflow(t, "AGT-CB-6")

	wfiID := env.GetWorkflowInstanceID(t, "AGT-CB-6", "test")
	env.InsertAgentSession(t, "sess-cb-6", "AGT-CB-6", wfiID, "analyzer", "analyzer", "sonnet")

	// Set result to callback
	env.MustExecute(t, "agent.callback", map[string]interface{}{
		"ticket_id":   "AGT-CB-6",
		"workflow":    "test",
		"agent_type":  "analyzer",
		"session_id":  "sess-cb-6",
		"level":       0,
		"instance_id": wfiID,
	}, nil)

	// Simulate spawner setting status to callback via UpdateSessionStatus
	err := env.AgentSvc.UpdateSessionStatus("sess-cb-6", model.AgentSessionCallback)
	if err != nil {
		t.Fatalf("failed to update session status: %v", err)
	}

	// Verify session status is "callback"
	session, err := env.AgentSvc.GetSessionByID("sess-cb-6")
	if err != nil {
		t.Fatalf("failed to get session: %v", err)
	}
	if session.Status != model.AgentSessionCallback {
		t.Fatalf("expected status 'callback', got %v", session.Status)
	}
}

func TestAgentCallbackE2E(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "AGT-CB-E2E", "Agent callback end-to-end")
	env.InitWorkflow(t, "AGT-CB-E2E")

	wfiID := env.GetWorkflowInstanceID(t, "AGT-CB-E2E", "test")
	env.InsertAgentSession(t, "sess-cb-e2e", "AGT-CB-E2E", wfiID, "analyzer", "analyzer", "sonnet")

	// 1. Agent saves callback_instructions finding
	env.MustExecute(t, "findings.add-bulk", map[string]interface{}{
		"ticket_id":  "AGT-CB-E2E",
		"workflow":   "test",
		"agent_type": "analyzer",
		"key_values": map[string]interface{}{
			"callback_instructions": "The implementation has a bug. Need to fix variable naming in layer 0.",
			"files_affected":        `["main.go","handler.go"]`,
		},
		"instance_id": wfiID,
	}, nil)

	// 2. Agent calls agent.callback with level
	env.MustExecute(t, "agent.callback", map[string]interface{}{
		"ticket_id":   "AGT-CB-E2E",
		"workflow":    "test",
		"agent_type":  "analyzer",
		"session_id":  "sess-cb-e2e",
		"level":       0,
		"instance_id": wfiID,
	}, nil)

	// 3. Verify result is callback
	session, err := env.AgentSvc.GetSessionByID("sess-cb-e2e")
	if err != nil {
		t.Fatalf("failed to get session: %v", err)
	}
	if session.Result.String != "callback" {
		t.Fatalf("expected result 'callback', got %v", session.Result.String)
	}

	// 4. Verify callback_level is saved by the CLI command
	findings := session.GetFindings()
	level := findings["callback_level"].(float64)
	if int(level) != 0 {
		t.Fatalf("expected callback_level to be 0, got %v", level)
	}

	// 5. Verify callback_instructions is present
	instructions := findings["callback_instructions"]
	if instructions == nil {
		t.Fatalf("expected callback_instructions to be present")
	}

	// 6. Simulate spawner detecting callback result and setting status
	err = env.AgentSvc.UpdateSessionStatus("sess-cb-e2e", model.AgentSessionCallback)
	if err != nil {
		t.Fatalf("failed to update session status: %v", err)
	}

	// 7. Verify final status is callback
	session, err = env.AgentSvc.GetSessionByID("sess-cb-e2e")
	if err != nil {
		t.Fatalf("failed to get session: %v", err)
	}
	if session.Status != model.AgentSessionCallback {
		t.Fatalf("expected final status 'callback', got %v", session.Status)
	}
	if session.Result.String != "callback" {
		t.Fatalf("expected final result 'callback', got %v", session.Result.String)
	}
}

func TestAgentCallbackRequestUnmarshal(t *testing.T) {
	// Test that AgentCallbackRequest correctly embeds AgentRequest and includes Level
	reqJSON := `{
		"workflow": "test",
		"agent_type": "analyzer",
		"model": "sonnet",
		"level": 2
	}`

	var req types.AgentCallbackRequest
	err := json.Unmarshal([]byte(reqJSON), &req)
	if err != nil {
		t.Fatalf("failed to unmarshal AgentCallbackRequest: %v", err)
	}

	if req.Workflow != "test" {
		t.Fatalf("expected workflow 'test', got %v", req.Workflow)
	}
	if req.AgentType != "analyzer" {
		t.Fatalf("expected agent_type 'analyzer', got %v", req.AgentType)
	}
	if req.Model != "sonnet" {
		t.Fatalf("expected model 'sonnet', got %v", req.Model)
	}
	if req.Level != 2 {
		t.Fatalf("expected level 2, got %v", req.Level)
	}
}

func TestAgentCallbackDifferentLevels(t *testing.T) {
	env := NewTestEnv(t)

	testCases := []struct {
		name     string
		ticketID string
		level    int
	}{
		{"Level 0", "AGT-CB-L0", 0},
		{"Level 1", "AGT-CB-L1", 1},
		{"Level 5", "AGT-CB-L5", 5},
		{"Level 10", "AGT-CB-L10", 10},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			env.CreateTicket(t, tc.ticketID, "Agent callback level test")
			env.InitWorkflow(t, tc.ticketID)

			wfiID := env.GetWorkflowInstanceID(t, tc.ticketID, "test")
			sessionID := "sess-" + tc.ticketID
			env.InsertAgentSession(t, sessionID, tc.ticketID, wfiID, "analyzer", "analyzer", "sonnet")

			// Call agent.callback with specific level
			env.MustExecute(t, "agent.callback", map[string]interface{}{
				"ticket_id":   tc.ticketID,
				"workflow":    "test",
				"agent_type":  "analyzer",
				"session_id":  sessionID,
				"level":       tc.level,
				"instance_id": wfiID,
			}, nil)

			// Verify callback_level
			session, err := env.AgentSvc.GetSessionByID(sessionID)
			if err != nil {
				t.Fatalf("failed to get session: %v", err)
			}

			findings := session.GetFindings()
			levelFloat := findings["callback_level"].(float64)
			if int(levelFloat) != tc.level {
				t.Fatalf("expected callback_level to be %d, got %v", tc.level, levelFloat)
			}
		})
	}
}
