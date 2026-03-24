package service

import (
	"testing"

	"be/internal/types"
)

// --- CreateAgentDef low_consumption_agent ---

func TestCreateAgentDef_WithLowConsumptionAgent(t *testing.T) {
	_, svc, wfID := setupAgentDefTestEnv(t, nil)

	// Create the referenced agent first
	_, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:     "agent-a",
		Prompt: "agent a",
	})
	if err != nil {
		t.Fatalf("create agent-a: %v", err)
	}

	def, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:                  "agent-b",
		Prompt:              "agent b",
		LowConsumptionAgent: "agent-a",
	})
	if err != nil {
		t.Fatalf("CreateAgentDef with low_consumption_agent: %v", err)
	}
	if def.LowConsumptionAgent != "agent-a" {
		t.Errorf("LowConsumptionAgent = %q, want %q", def.LowConsumptionAgent, "agent-a")
	}
}

func TestGetAgentDef_ReturnsLowConsumptionAgent(t *testing.T) {
	_, svc, wfID := setupAgentDefTestEnv(t, nil)

	_, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:     "ref-agent",
		Prompt: "ref",
	})
	if err != nil {
		t.Fatalf("create ref-agent: %v", err)
	}

	_, err = svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:                  "main-agent",
		Prompt:              "main",
		LowConsumptionAgent: "ref-agent",
	})
	if err != nil {
		t.Fatalf("create main-agent: %v", err)
	}

	got, err := svc.GetAgentDef("proj1", wfID, "main-agent")
	if err != nil {
		t.Fatalf("GetAgentDef: %v", err)
	}
	if got.LowConsumptionAgent != "ref-agent" {
		t.Errorf("GetAgentDef LowConsumptionAgent = %q, want %q", got.LowConsumptionAgent, "ref-agent")
	}
}

func TestListAgentDefs_ReturnsLowConsumptionAgent(t *testing.T) {
	_, svc, wfID := setupAgentDefTestEnv(t, nil)

	_, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:     "la-ref",
		Prompt: "ref",
	})
	if err != nil {
		t.Fatalf("create la-ref: %v", err)
	}

	_, err = svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:                  "la-main",
		Prompt:              "main",
		LowConsumptionAgent: "la-ref",
	})
	if err != nil {
		t.Fatalf("create la-main: %v", err)
	}

	defs, err := svc.ListAgentDefs("proj1", wfID)
	if err != nil {
		t.Fatalf("ListAgentDefs: %v", err)
	}
	if len(defs) != 2 {
		t.Fatalf("expected 2 agent defs, got %d", len(defs))
	}

	// defs are ordered by id; la-main < la-ref
	if defs[0].LowConsumptionAgent != "la-ref" {
		t.Errorf("ListAgentDefs[0].LowConsumptionAgent = %q, want %q", defs[0].LowConsumptionAgent, "la-ref")
	}
	if defs[1].LowConsumptionAgent != "" {
		t.Errorf("ListAgentDefs[1].LowConsumptionAgent = %q, want empty", defs[1].LowConsumptionAgent)
	}
}

func TestUpdateAgentDef_UpdatesLowConsumptionAgent(t *testing.T) {
	_, svc, wfID := setupAgentDefTestEnv(t, nil)

	_, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{ID: "upd-ref", Prompt: "ref"})
	if err != nil {
		t.Fatalf("create upd-ref: %v", err)
	}
	_, err = svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{ID: "upd-main", Prompt: "main"})
	if err != nil {
		t.Fatalf("create upd-main: %v", err)
	}

	lcAgent := "upd-ref"
	if err := svc.UpdateAgentDef("proj1", wfID, "upd-main", &types.AgentDefUpdateRequest{
		LowConsumptionAgent: &lcAgent,
	}); err != nil {
		t.Fatalf("UpdateAgentDef: %v", err)
	}

	def, err := svc.GetAgentDef("proj1", wfID, "upd-main")
	if err != nil {
		t.Fatalf("GetAgentDef after update: %v", err)
	}
	if def.LowConsumptionAgent != "upd-ref" {
		t.Errorf("after update LowConsumptionAgent = %q, want %q", def.LowConsumptionAgent, "upd-ref")
	}
}

func TestUpdateAgentDef_ClearsLowConsumptionAgent(t *testing.T) {
	_, svc, wfID := setupAgentDefTestEnv(t, nil)

	_, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{ID: "clr-ref", Prompt: "ref"})
	if err != nil {
		t.Fatalf("create clr-ref: %v", err)
	}
	_, err = svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:                  "clr-main",
		Prompt:              "main",
		LowConsumptionAgent: "clr-ref",
	})
	if err != nil {
		t.Fatalf("create clr-main: %v", err)
	}

	empty := ""
	if err := svc.UpdateAgentDef("proj1", wfID, "clr-main", &types.AgentDefUpdateRequest{
		LowConsumptionAgent: &empty,
	}); err != nil {
		t.Fatalf("UpdateAgentDef clear: %v", err)
	}

	def, err := svc.GetAgentDef("proj1", wfID, "clr-main")
	if err != nil {
		t.Fatalf("GetAgentDef after clear: %v", err)
	}
	if def.LowConsumptionAgent != "" {
		t.Errorf("after clear LowConsumptionAgent = %q, want empty", def.LowConsumptionAgent)
	}
}

// --- Validation: self-reference ---

func TestCreateAgentDef_SelfReferenceLowConsumption(t *testing.T) {
	_, svc, wfID := setupAgentDefTestEnv(t, nil)

	_, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:                  "self-ref",
		Prompt:              "self",
		LowConsumptionAgent: "self-ref",
	})
	if err == nil {
		t.Fatal("expected error for self-referential low_consumption_agent, got nil")
	}
}

func TestUpdateAgentDef_SelfReferenceLowConsumption(t *testing.T) {
	_, svc, wfID := setupAgentDefTestEnv(t, nil)

	_, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:     "upd-self",
		Prompt: "self",
	})
	if err != nil {
		t.Fatalf("create upd-self: %v", err)
	}

	selfRef := "upd-self"
	if err := svc.UpdateAgentDef("proj1", wfID, "upd-self", &types.AgentDefUpdateRequest{
		LowConsumptionAgent: &selfRef,
	}); err == nil {
		t.Fatal("expected error for self-referential low_consumption_agent update, got nil")
	}
}

// --- Validation: non-existent reference ---

func TestCreateAgentDef_NonExistentLowConsumption(t *testing.T) {
	_, svc, wfID := setupAgentDefTestEnv(t, nil)

	_, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:                  "orphan",
		Prompt:              "orphan",
		LowConsumptionAgent: "does-not-exist",
	})
	if err == nil {
		t.Fatal("expected error for non-existent low_consumption_agent, got nil")
	}
}

func TestUpdateAgentDef_NonExistentLowConsumption(t *testing.T) {
	_, svc, wfID := setupAgentDefTestEnv(t, nil)

	_, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:     "upd-orphan",
		Prompt: "orphan",
	})
	if err != nil {
		t.Fatalf("create upd-orphan: %v", err)
	}

	missing := "does-not-exist"
	if err := svc.UpdateAgentDef("proj1", wfID, "upd-orphan", &types.AgentDefUpdateRequest{
		LowConsumptionAgent: &missing,
	}); err == nil {
		t.Fatal("expected error for non-existent low_consumption_agent update, got nil")
	}
}

// --- Casing: value is lowercased on create and update ---

func TestCreateAgentDef_LowConsumptionAgentLowercased(t *testing.T) {
	_, svc, wfID := setupAgentDefTestEnv(t, nil)

	_, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:     "lc-lower-ref",
		Prompt: "ref",
	})
	if err != nil {
		t.Fatalf("create lc-lower-ref: %v", err)
	}

	def, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:                  "lc-lower-main",
		Prompt:              "main",
		LowConsumptionAgent: "LC-LOWER-REF", // upper case
	})
	if err != nil {
		t.Fatalf("CreateAgentDef with mixed-case low_consumption_agent: %v", err)
	}
	if def.LowConsumptionAgent != "lc-lower-ref" {
		t.Errorf("LowConsumptionAgent = %q, want %q", def.LowConsumptionAgent, "lc-lower-ref")
	}
}

func TestUpdateAgentDef_LowConsumptionAgentLowercased(t *testing.T) {
	_, svc, wfID := setupAgentDefTestEnv(t, nil)

	_, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{ID: "case-ref", Prompt: "ref"})
	if err != nil {
		t.Fatalf("create case-ref: %v", err)
	}
	_, err = svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{ID: "case-main", Prompt: "main"})
	if err != nil {
		t.Fatalf("create case-main: %v", err)
	}

	mixed := "CASE-REF"
	if err := svc.UpdateAgentDef("proj1", wfID, "case-main", &types.AgentDefUpdateRequest{
		LowConsumptionAgent: &mixed,
	}); err != nil {
		t.Fatalf("UpdateAgentDef: %v", err)
	}

	def, err := svc.GetAgentDef("proj1", wfID, "case-main")
	if err != nil {
		t.Fatalf("GetAgentDef after update: %v", err)
	}
	if def.LowConsumptionAgent != "case-ref" {
		t.Errorf("after update LowConsumptionAgent = %q, want %q", def.LowConsumptionAgent, "case-ref")
	}
}

// --- Default: empty low_consumption_agent ---

func TestCreateAgentDef_DefaultEmptyLowConsumptionAgent(t *testing.T) {
	_, svc, wfID := setupAgentDefTestEnv(t, nil)

	def, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:     "no-lc",
		Prompt: "no lc",
	})
	if err != nil {
		t.Fatalf("CreateAgentDef: %v", err)
	}
	if def.LowConsumptionAgent != "" {
		t.Errorf("LowConsumptionAgent = %q, want empty", def.LowConsumptionAgent)
	}

	got, err := svc.GetAgentDef("proj1", wfID, "no-lc")
	if err != nil {
		t.Fatalf("GetAgentDef: %v", err)
	}
	if got.LowConsumptionAgent != "" {
		t.Errorf("GetAgentDef LowConsumptionAgent = %q, want empty", got.LowConsumptionAgent)
	}
}
