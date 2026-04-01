package model

import (
	"encoding/json"
	"testing"
	"time"
)

func baseSession() AgentSession {
	return AgentSession{
		ID:                 "sess-1",
		ProjectID:          "proj",
		TicketID:           "TKT-1",
		WorkflowInstanceID: "wfi-1",
		Phase:              "phase0",
		AgentType:          "test-agent",
		Status:             AgentSessionRunning,
		CreatedAt:          time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		UpdatedAt:          time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}
}

// TestAgentSessionMarshalJSON_ConfigOmittedWhenEmpty verifies config field is
// omitted from JSON when empty string (omitempty).
func TestAgentSessionMarshalJSON_ConfigOmittedWhenEmpty(t *testing.T) {
	sess := baseSession()
	sess.Config = ""

	data, err := json.Marshal(sess)
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}

	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if _, exists := m["config"]; exists {
		t.Errorf("config field should be omitted when empty, but was present in JSON")
	}
}

// TestAgentSessionMarshalJSON_ConfigIncludedWhenSet verifies config field appears
// in JSON when non-empty.
func TestAgentSessionMarshalJSON_ConfigIncludedWhenSet(t *testing.T) {
	sess := baseSession()
	sess.Config = `{"allowedTools":["bash"]}`

	data, err := json.Marshal(sess)
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}

	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	got, exists := m["config"]
	if !exists {
		t.Fatal("config field missing from JSON when non-empty")
	}
	if got != `{"allowedTools":["bash"]}` {
		t.Errorf("config = %v, want %q", got, `{"allowedTools":["bash"]}`)
	}
}

// TestAgentSessionMarshalJSON_ConfigPreservesExactString verifies the config
// value is stored as a plain string (not re-parsed as JSON).
func TestAgentSessionMarshalJSON_ConfigPreservesExactString(t *testing.T) {
	sess := baseSession()
	// A JSON-looking string value.
	sess.Config = `{"permissions":{"allow":["*"]},"safety":true}`

	data, err := json.Marshal(sess)
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}

	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	got, ok := m["config"].(string)
	if !ok {
		t.Fatalf("config field is not a string: %T", m["config"])
	}
	if got != sess.Config {
		t.Errorf("config = %q, want %q", got, sess.Config)
	}
}

// TestAgentSessionMarshalJSON_RestartCountAlwaysPresent verifies restart_count
// field remains in the JSON output (not omitted when zero) since it's not omitempty.
func TestAgentSessionMarshalJSON_RestartCountAlwaysPresent(t *testing.T) {
	sess := baseSession()
	sess.RestartCount = 0

	data, err := json.Marshal(sess)
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}

	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if _, exists := m["restart_count"]; !exists {
		t.Error("restart_count should be present even when zero")
	}
}

// TestAgentSessionMarshalJSON_RequiredFieldsPresent verifies core fields are
// always serialized regardless of Config value.
func TestAgentSessionMarshalJSON_RequiredFieldsPresent(t *testing.T) {
	for _, cfg := range []string{"", `{"key":"val"}`} {
		t.Run("config="+cfg, func(t *testing.T) {
			sess := baseSession()
			sess.Config = cfg

			data, err := json.Marshal(sess)
			if err != nil {
				t.Fatalf("MarshalJSON: %v", err)
			}

			var m map[string]interface{}
			if err := json.Unmarshal(data, &m); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}

			for _, field := range []string{"id", "project_id", "ticket_id", "status", "phase", "agent_type"} {
				if _, exists := m[field]; !exists {
					t.Errorf("field %q missing from JSON output", field)
				}
			}
		})
	}
}
