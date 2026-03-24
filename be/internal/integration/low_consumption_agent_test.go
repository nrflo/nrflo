package integration

import (
	"encoding/json"
	"testing"

	"be/internal/types"
)

// TestAgentDefCreateWithLowConsumptionAgent verifies that creating an agent definition
// with low_consumption_agent persists and returns it correctly.
func TestAgentDefCreateWithLowConsumptionAgent(t *testing.T) {
	env := NewTestEnv(t)
	svc := env.getAgentDefService(t)

	// Create the referenced agent first
	_, err := svc.CreateAgentDef(env.ProjectID, "test", &types.AgentDefCreateRequest{
		ID:     "lc-ref",
		Prompt: "reference agent",
	})
	if err != nil {
		t.Fatalf("create ref agent: %v", err)
	}

	def, err := svc.CreateAgentDef(env.ProjectID, "test", &types.AgentDefCreateRequest{
		ID:                  "lc-main",
		Prompt:              "main agent",
		LowConsumptionAgent: "lc-ref",
	})
	if err != nil {
		t.Fatalf("create main agent with low_consumption_agent: %v", err)
	}

	if def.LowConsumptionAgent != "lc-ref" {
		t.Errorf("LowConsumptionAgent = %q, want %q", def.LowConsumptionAgent, "lc-ref")
	}
}

// TestAgentDefGetLowConsumptionAgent verifies that GET returns low_consumption_agent correctly.
func TestAgentDefGetLowConsumptionAgent(t *testing.T) {
	env := NewTestEnv(t)
	svc := env.getAgentDefService(t)

	_, err := svc.CreateAgentDef(env.ProjectID, "test", &types.AgentDefCreateRequest{
		ID:     "get-ref",
		Prompt: "ref",
	})
	if err != nil {
		t.Fatalf("create ref: %v", err)
	}
	_, err = svc.CreateAgentDef(env.ProjectID, "test", &types.AgentDefCreateRequest{
		ID:                  "get-main",
		Prompt:              "main",
		LowConsumptionAgent: "get-ref",
	})
	if err != nil {
		t.Fatalf("create main: %v", err)
	}

	got, err := svc.GetAgentDef(env.ProjectID, "test", "get-main")
	if err != nil {
		t.Fatalf("GetAgentDef: %v", err)
	}
	if got.LowConsumptionAgent != "get-ref" {
		t.Errorf("GetAgentDef LowConsumptionAgent = %q, want %q", got.LowConsumptionAgent, "get-ref")
	}
}

// TestAgentDefEmptyLowConsumptionAgentOmittedFromJSON verifies that when
// low_consumption_agent is empty, it is omitted from the JSON output via omitempty.
func TestAgentDefEmptyLowConsumptionAgentOmittedFromJSON(t *testing.T) {
	env := NewTestEnv(t)
	svc := env.getAgentDefService(t)

	def, err := svc.CreateAgentDef(env.ProjectID, "test", &types.AgentDefCreateRequest{
		ID:     "no-lc-agent",
		Prompt: "no low consumption",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if def.LowConsumptionAgent != "" {
		t.Fatalf("expected LowConsumptionAgent to be empty, got %q", def.LowConsumptionAgent)
	}

	data, err := json.Marshal(def)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, exists := result["low_consumption_agent"]; exists {
		t.Fatal("expected low_consumption_agent to be omitted from JSON when empty")
	}
}

// TestAgentDefUpdateLowConsumptionAgent verifies that updating low_consumption_agent works end-to-end.
func TestAgentDefUpdateLowConsumptionAgent(t *testing.T) {
	env := NewTestEnv(t)
	svc := env.getAgentDefService(t)

	_, err := svc.CreateAgentDef(env.ProjectID, "test", &types.AgentDefCreateRequest{
		ID:     "upd-lc-ref",
		Prompt: "ref",
	})
	if err != nil {
		t.Fatalf("create ref: %v", err)
	}
	_, err = svc.CreateAgentDef(env.ProjectID, "test", &types.AgentDefCreateRequest{
		ID:     "upd-lc-main",
		Prompt: "main",
	})
	if err != nil {
		t.Fatalf("create main: %v", err)
	}

	lcAgent := "upd-lc-ref"
	if err := svc.UpdateAgentDef(env.ProjectID, "test", "upd-lc-main", &types.AgentDefUpdateRequest{
		LowConsumptionAgent: &lcAgent,
	}); err != nil {
		t.Fatalf("UpdateAgentDef: %v", err)
	}

	def, err := svc.GetAgentDef(env.ProjectID, "test", "upd-lc-main")
	if err != nil {
		t.Fatalf("GetAgentDef after update: %v", err)
	}
	if def.LowConsumptionAgent != "upd-lc-ref" {
		t.Errorf("after update LowConsumptionAgent = %q, want %q", def.LowConsumptionAgent, "upd-lc-ref")
	}
}
