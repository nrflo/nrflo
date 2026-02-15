package ws

import (
	"encoding/json"
	"testing"
	"time"
)

// TestEventSchemaCommonFields verifies all events have required common fields.
func TestEventSchemaCommonFields(t *testing.T) {
	tests := []struct {
		name      string
		eventType string
		data      map[string]interface{}
	}{
		{
			name:      "agent.started",
			eventType: EventAgentStarted,
			data:      map[string]interface{}{"agent_type": "test", "session_id": "s1"},
		},
		{
			name:      "agent.completed",
			eventType: EventAgentCompleted,
			data:      map[string]interface{}{"agent_type": "test", "session_id": "s1"},
		},
		{
			name:      "agent.continued",
			eventType: EventAgentContinued,
			data:      map[string]interface{}{"agent_type": "test", "session_id": "s1"},
		},
		{
			name:      "phase.started",
			eventType: EventPhaseStarted,
			data:      map[string]interface{}{"phase": "test"},
		},
		{
			name:      "phase.completed",
			eventType: EventPhaseCompleted,
			data:      map[string]interface{}{"phase": "test"},
		},
		{
			name:      "findings.updated",
			eventType: EventFindingsUpdated,
			data:      map[string]interface{}{"agent_type": "test", "key": "test"},
		},
		{
			name:      "messages.updated",
			eventType: EventMessagesUpdated,
			data:      map[string]interface{}{"session_id": "s1"},
		},
		{
			name:      "workflow.updated",
			eventType: EventWorkflowUpdated,
			data:      map[string]interface{}{"status": "active"},
		},
		{
			name:      "chain.updated",
			eventType: EventChainUpdated,
			data:      map[string]interface{}{"chain_id": "c1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := NewEvent(tt.eventType, "proj-1", "ticket-1", "test", tt.data)

			// Required common fields
			if event.Type != tt.eventType {
				t.Errorf("expected type %s, got %s", tt.eventType, event.Type)
			}
			if event.ProjectID != "proj-1" {
				t.Errorf("expected project_id proj-1, got %s", event.ProjectID)
			}
			// Note: Timestamp is set by Hub.broadcastEvent(), not NewEvent()
			if event.Timestamp != "" {
				t.Error("timestamp should not be set by NewEvent (set by broadcastEvent)")
			}
			// Data field should match input
			if event.Data == nil && tt.data != nil {
				t.Error("data field should not be nil when provided")
			}
		})
	}
}

// TestEventAgentStartedPayload verifies agent.started events have required fields.
func TestEventAgentStartedPayload(t *testing.T) {
	event := NewEvent(EventAgentStarted, "proj-1", "ticket-1", "test", map[string]interface{}{
		"agent_type": "implementor",
		"session_id": "sess-123",
		"model_id":   "claude-opus-4",
	})

	if event.Type != EventAgentStarted {
		t.Errorf("expected type %s, got %s", EventAgentStarted, event.Type)
	}

	// Verify event-specific fields
	agentType, ok := event.Data["agent_type"].(string)
	if !ok || agentType != "implementor" {
		t.Errorf("expected agent_type=implementor, got %v", event.Data["agent_type"])
	}

	sessionID, ok := event.Data["session_id"].(string)
	if !ok || sessionID != "sess-123" {
		t.Errorf("expected session_id=sess-123, got %v", event.Data["session_id"])
	}

	modelID, ok := event.Data["model_id"].(string)
	if !ok || modelID != "claude-opus-4" {
		t.Errorf("expected model_id=claude-opus-4, got %v", event.Data["model_id"])
	}
}

// TestEventAgentCompletedPayload verifies agent.completed events have required fields.
func TestEventAgentCompletedPayload(t *testing.T) {
	tests := []struct {
		name   string
		result string
	}{
		{"pass", "pass"},
		{"fail", "fail"},
		{"callback", "callback"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := NewEvent(EventAgentCompleted, "proj-1", "ticket-1", "test", map[string]interface{}{
				"action":     tt.result,
				"agent_type": "implementor",
				"session_id": "sess-123",
				"model_id":   "claude-opus-4",
				"result":     tt.result,
			})

			if event.Type != EventAgentCompleted {
				t.Errorf("expected type %s, got %s", EventAgentCompleted, event.Type)
			}

			// Verify event-specific fields
			result, ok := event.Data["result"].(string)
			if !ok || result != tt.result {
				t.Errorf("expected result=%s, got %v", tt.result, event.Data["result"])
			}

			sessionID, ok := event.Data["session_id"].(string)
			if !ok || sessionID == "" {
				t.Errorf("session_id must be present, got %v", event.Data["session_id"])
			}

			modelID, ok := event.Data["model_id"].(string)
			if !ok || modelID == "" {
				t.Errorf("model_id must be present, got %v", event.Data["model_id"])
			}
		})
	}
}

// TestEventAgentContinuedPayload verifies agent.continued events have required fields.
func TestEventAgentContinuedPayload(t *testing.T) {
	event := NewEvent(EventAgentContinued, "proj-1", "ticket-1", "test", map[string]interface{}{
		"agent_type": "implementor",
		"session_id": "sess-new",
		"model_id":   "claude-opus-4",
	})

	if event.Type != EventAgentContinued {
		t.Errorf("expected type %s, got %s", EventAgentContinued, event.Type)
	}

	sessionID, ok := event.Data["session_id"].(string)
	if !ok || sessionID == "" {
		t.Errorf("session_id must be present, got %v", event.Data["session_id"])
	}

	modelID, ok := event.Data["model_id"].(string)
	if !ok || modelID == "" {
		t.Errorf("model_id must be present, got %v", event.Data["model_id"])
	}
}

// TestEventMessagesUpdatedPayload verifies messages.updated events have required fields.
func TestEventMessagesUpdatedPayload(t *testing.T) {
	event := NewEvent(EventMessagesUpdated, "proj-1", "ticket-1", "test", map[string]interface{}{
		"session_id": "sess-123",
		"agent_type": "implementor",
		"model_id":   "claude-opus-4",
	})

	if event.Type != EventMessagesUpdated {
		t.Errorf("expected type %s, got %s", EventMessagesUpdated, event.Type)
	}

	sessionID, ok := event.Data["session_id"].(string)
	if !ok || sessionID == "" {
		t.Errorf("session_id must be present, got %v", event.Data["session_id"])
	}

	agentType, ok := event.Data["agent_type"].(string)
	if !ok || agentType == "" {
		t.Errorf("agent_type must be present, got %v", event.Data["agent_type"])
	}

	modelID, ok := event.Data["model_id"].(string)
	if !ok || modelID == "" {
		t.Errorf("model_id must be present, got %v", event.Data["model_id"])
	}
}

// TestEventChainUpdatedPayload verifies chain.updated events have required fields.
func TestEventChainUpdatedPayload(t *testing.T) {
	event := NewEvent(EventChainUpdated, "proj-1", "", "test", map[string]interface{}{
		"chain_id": "chain-123",
		"status":   "running",
	})

	if event.Type != EventChainUpdated {
		t.Errorf("expected type %s, got %s", EventChainUpdated, event.Type)
	}

	chainID, ok := event.Data["chain_id"].(string)
	if !ok || chainID == "" {
		t.Errorf("chain_id must be present, got %v", event.Data["chain_id"])
	}
}

// TestEventSerialization verifies events can be marshaled to JSON correctly.
func TestEventSerialization(t *testing.T) {
	event := NewEvent(EventAgentCompleted, "proj-1", "ticket-1", "test", map[string]interface{}{
		"session_id": "sess-123",
		"result":     "pass",
	})
	// Manually set timestamp to simulate what broadcastEvent does
	event.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("failed to marshal event: %v", err)
	}

	var decoded Event
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal event: %v", err)
	}

	if decoded.Type != event.Type {
		t.Errorf("type mismatch: expected %s, got %s", event.Type, decoded.Type)
	}
	if decoded.ProjectID != event.ProjectID {
		t.Errorf("project_id mismatch: expected %s, got %s", event.ProjectID, decoded.ProjectID)
	}
	if decoded.TicketID != event.TicketID {
		t.Errorf("ticket_id mismatch: expected %s, got %s", event.TicketID, decoded.TicketID)
	}
	if decoded.Timestamp != event.Timestamp {
		t.Errorf("timestamp mismatch: expected %s, got %s", event.Timestamp, decoded.Timestamp)
	}
}

// TestEventConstantsExist verifies all event constants are defined.
func TestEventConstantsExist(t *testing.T) {
	constants := []struct {
		name  string
		value string
	}{
		{"EventAgentStarted", EventAgentStarted},
		{"EventAgentCompleted", EventAgentCompleted},
		{"EventAgentContinued", EventAgentContinued},
		{"EventPhaseStarted", EventPhaseStarted},
		{"EventPhaseCompleted", EventPhaseCompleted},
		{"EventFindingsUpdated", EventFindingsUpdated},
		{"EventMessagesUpdated", EventMessagesUpdated},
		{"EventWorkflowUpdated", EventWorkflowUpdated},
		{"EventChainUpdated", EventChainUpdated},
	}

	for _, c := range constants {
		if c.value == "" {
			t.Errorf("constant %s is empty", c.name)
		}
		// Verify format is "resource.action"
		if len(c.value) < 3 || !contains(c.value, ".") {
			t.Errorf("constant %s has invalid format: %s (expected 'resource.action')", c.name, c.value)
		}
	}
}

func contains(s, substr string) bool {
	for i := 0; i < len(s); i++ {
		if i+len(substr) <= len(s) && s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
