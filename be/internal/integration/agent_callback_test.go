package integration

import (
	"encoding/json"
	"testing"

	"be/internal/clock"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/types"
)

// getSessionFindings returns the findings for a session as map[string]interface{},
// matching the semantics of the old session.GetFindings() method.
func getSessionFindings(t *testing.T, env *TestEnv, sessionID string) map[string]interface{} {
	t.Helper()
	fr := repo.NewFindingRepo(env.Pool, clock.Real())
	raw, err := fr.GetOwn("session", sessionID)
	if err != nil {
		t.Fatalf("getSessionFindings: %v", err)
	}
	result := make(map[string]interface{})
	for k, v := range raw {
		var val interface{}
		if err := json.Unmarshal(v, &val); err != nil {
			t.Fatalf("getSessionFindings: unmarshal %s: %v", k, err)
		}
		result[k] = val
	}
	return result
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
		"level":       1,
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
		"session_id":  "sess-cb-5",
		"instance_id": wfiID,
		"key_values": map[string]interface{}{
			"callback_instructions": "Fix the bug in layer 0",
			"bug_description":       "NPE in handler",
		},
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

	findings := getSessionFindings(t, env, session.ID)
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
		"level":       1,
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
		"session_id":  "sess-cb-e2e",
		"instance_id": wfiID,
		"key_values": map[string]interface{}{
			"callback_instructions": "The implementation has a bug. Need to fix variable naming in layer 0.",
			"files_affected":        `["main.go","handler.go"]`,
		},
	}, nil)

	// 2. Agent calls agent.callback with level
	env.MustExecute(t, "agent.callback", map[string]interface{}{
		"ticket_id":   "AGT-CB-E2E",
		"workflow":    "test",
		"agent_type":  "analyzer",
		"session_id":  "sess-cb-e2e",
		"level":       1,
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
	findings := getSessionFindings(t, env, session.ID)
	level := findings["callback_level"].(float64)
	if int(level) != 1 {
		t.Fatalf("expected callback_level to be 1, got %v", level)
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
		"session_id": "sess-abc",
		"instance_id": "wfi-xyz",
		"level": 2
	}`

	var req types.AgentCallbackRequest
	err := json.Unmarshal([]byte(reqJSON), &req)
	if err != nil {
		t.Fatalf("failed to unmarshal AgentCallbackRequest: %v", err)
	}

	if req.SessionID != "sess-abc" {
		t.Fatalf("expected session_id 'sess-abc', got %v", req.SessionID)
	}
	if req.InstanceID != "wfi-xyz" {
		t.Fatalf("expected instance_id 'wfi-xyz', got %v", req.InstanceID)
	}
	if req.Level != 2 {
		t.Fatalf("expected level 2, got %v", req.Level)
	}
}

func TestAgentCallbackDifferentLevels(t *testing.T) {
	env := NewTestEnv(t)

	testCases := []struct {
		name        string
		ticketID    string
		agentType   string
		cliModel    string
		modelFilter string // sent as the "model" param when non-empty
		level       int
		// wantLayerMode asserts the callback_mode='layer' finding (only valid
		// for level 0, the default-layer-mode zero value).
		wantLayerMode bool
	}{
		{name: "Level 0", ticketID: "AGT-CB-L0", agentType: "analyzer", cliModel: "haiku", level: 0, wantLayerMode: true},
		{name: "Level 1", ticketID: "AGT-CB-L1", agentType: "analyzer", cliModel: "sonnet", level: 1},
		{name: "Level 2 with model filter", ticketID: "AGT-CB-L2", agentType: "builder", cliModel: "opus_4_7", modelFilter: "opus_4_7", level: 2},
		{name: "Level 5", ticketID: "AGT-CB-L5", agentType: "analyzer", cliModel: "sonnet", level: 5},
		{name: "Level 10", ticketID: "AGT-CB-L10", agentType: "analyzer", cliModel: "sonnet", level: 10},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			env.CreateTicket(t, tc.ticketID, "Agent callback level test")
			env.InitWorkflow(t, tc.ticketID)

			wfiID := env.GetWorkflowInstanceID(t, tc.ticketID, "test")
			sessionID := "sess-" + tc.ticketID
			env.InsertAgentSession(t, sessionID, tc.ticketID, wfiID, tc.agentType, tc.agentType, tc.cliModel)

			// Call agent.callback with specific level (and model filter if set)
			params := map[string]interface{}{
				"ticket_id":   tc.ticketID,
				"workflow":    "test",
				"agent_type":  tc.agentType,
				"session_id":  sessionID,
				"level":       tc.level,
				"instance_id": wfiID,
			}
			if tc.modelFilter != "" {
				params["model"] = tc.modelFilter
			}
			var result map[string]string
			env.MustExecute(t, "agent.callback", params, &result)

			if result["status"] != "callback" {
				t.Fatalf("expected status=callback, got %q", result["status"])
			}

			// Verify session result is "callback"
			session, err := env.AgentSvc.GetSessionByID(sessionID)
			if err != nil {
				t.Fatalf("failed to get session: %v", err)
			}
			if session.Result.String != "callback" {
				t.Fatalf("expected result 'callback', got %v", session.Result.String)
			}

			// Verify callback_level finding (JSON unmarshals numbers to float64)
			findings := getSessionFindings(t, env, session.ID)
			level, ok := findings["callback_level"]
			if !ok {
				t.Fatalf("expected callback_level finding to be set")
			}
			levelFloat, ok := level.(float64)
			if !ok {
				t.Fatalf("expected callback_level to be a number, got %T", level)
			}
			if int(levelFloat) != tc.level {
				t.Fatalf("expected callback_level to be %d, got %v", tc.level, levelFloat)
			}

			if tc.wantLayerMode {
				if got, ok := findings["callback_mode"].(string); !ok || got != "layer" {
					t.Fatalf("expected callback_mode='layer' finding, got %v", findings["callback_mode"])
				}
			}
		})
	}
}
