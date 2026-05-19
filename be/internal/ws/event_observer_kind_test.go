package ws

import "testing"

// TestAgentStartedEvent_WorkflowAgentKind verifies agent.started payload carries kind=workflow_agent.
func TestAgentStartedEvent_WorkflowAgentKind(t *testing.T) {
	event := NewEvent(EventAgentStarted, "proj-1", "TKT-1", "feature", map[string]interface{}{
		"agent_id":          "agt-123",
		"agent_type":        "implementor",
		"model_id":          "sonnet",
		"session_id":        "sess-1",
		"phase":             "implementor",
		"restart_threshold": 3,
		"kind":              "workflow_agent",
	})

	kind, ok := event.Data["kind"].(string)
	if !ok {
		t.Fatalf("kind field missing or not a string in agent.started payload")
	}
	if kind != "workflow_agent" {
		t.Errorf("kind = %q, want workflow_agent", kind)
	}
}

// TestAgentStartedEvent_ObserverKind verifies agent.started payload carries kind=observer.
func TestAgentStartedEvent_ObserverKind(t *testing.T) {
	event := NewEvent(EventAgentStarted, "proj-1", "", "wf-obs", map[string]interface{}{
		"agent_id":          "obs-abc",
		"agent_type":        "_observer",
		"model_id":          "sonnet",
		"session_id":        "sess-obs-1",
		"phase":             "observer",
		"restart_threshold": 0,
		"kind":              "observer",
	})

	kind, ok := event.Data["kind"].(string)
	if !ok {
		t.Fatalf("kind field missing or not a string in observer agent.started payload")
	}
	if kind != "observer" {
		t.Errorf("kind = %q, want observer", kind)
	}
}

// TestAgentCompletedEvent_KindField verifies agent.completed payload carries the kind field.
func TestAgentCompletedEvent_KindField(t *testing.T) {
	cases := []struct {
		name string
		kind string
	}{
		{"workflow_agent", "workflow_agent"},
		{"observer", "observer"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			event := NewEvent(EventAgentCompleted, "proj-1", "TKT-1", "feature", map[string]interface{}{
				"agent_id":      "agt-1",
				"session_id":    "sess-1",
				"result":        "pass",
				"result_reason": "",
				"model_id":      "sonnet",
				"kind":          tc.kind,
			})

			kind, ok := event.Data["kind"].(string)
			if !ok {
				t.Fatalf("kind field missing in agent.completed payload")
			}
			if kind != tc.kind {
				t.Errorf("kind = %q, want %q", kind, tc.kind)
			}
		})
	}
}

// TestAgentStartedEvent_KindFieldRequired verifies a payload without kind is not a problem
// (the spawner always includes it, but the event struct doesn't enforce it).
func TestAgentStartedEvent_AllRequiredFieldsWithKind(t *testing.T) {
	requiredFields := []string{"agent_id", "agent_type", "model_id", "session_id", "phase", "restart_threshold", "kind"}

	event := NewEvent(EventAgentStarted, "proj-1", "TKT-1", "feature", map[string]interface{}{
		"agent_id":          "agt-1",
		"agent_type":        "implementor",
		"model_id":          "sonnet",
		"session_id":        "sess-1",
		"phase":             "implementor",
		"restart_threshold": 3,
		"kind":              "workflow_agent",
	})

	for _, field := range requiredFields {
		if _, ok := event.Data[field]; !ok {
			t.Errorf("agent.started payload missing required field %q", field)
		}
	}
}
