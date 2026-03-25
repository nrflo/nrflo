package service

import (
	"testing"

	"be/internal/types"
)

// --- CreateAgentDef low_consumption_model ---

func TestCreateAgentDef_WithLowConsumptionModel(t *testing.T) {
	_, svc, wfID := setupAgentDefTestEnv(t, nil)

	def, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:                  "agent-b",
		Prompt:              "agent b",
		LowConsumptionModel: "sonnet",
	})
	if err != nil {
		t.Fatalf("CreateAgentDef with low_consumption_model: %v", err)
	}
	if def.LowConsumptionModel != "sonnet" {
		t.Errorf("LowConsumptionModel = %q, want %q", def.LowConsumptionModel, "sonnet")
	}
}

func TestGetAgentDef_ReturnsLowConsumptionModel(t *testing.T) {
	_, svc, wfID := setupAgentDefTestEnv(t, nil)

	_, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:                  "main-agent",
		Prompt:              "main",
		LowConsumptionModel: "haiku",
	})
	if err != nil {
		t.Fatalf("create main-agent: %v", err)
	}

	got, err := svc.GetAgentDef("proj1", wfID, "main-agent")
	if err != nil {
		t.Fatalf("GetAgentDef: %v", err)
	}
	if got.LowConsumptionModel != "haiku" {
		t.Errorf("GetAgentDef LowConsumptionModel = %q, want %q", got.LowConsumptionModel, "haiku")
	}
}

func TestListAgentDefs_ReturnsLowConsumptionModel(t *testing.T) {
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
		LowConsumptionModel: "sonnet",
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
	if defs[0].LowConsumptionModel != "sonnet" {
		t.Errorf("ListAgentDefs[0].LowConsumptionModel = %q, want %q", defs[0].LowConsumptionModel, "sonnet")
	}
	if defs[1].LowConsumptionModel != "" {
		t.Errorf("ListAgentDefs[1].LowConsumptionModel = %q, want empty", defs[1].LowConsumptionModel)
	}
}

func TestUpdateAgentDef_UpdatesLowConsumptionModel(t *testing.T) {
	_, svc, wfID := setupAgentDefTestEnv(t, nil)

	_, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{ID: "upd-main", Prompt: "main"})
	if err != nil {
		t.Fatalf("create upd-main: %v", err)
	}

	lcModel := "haiku"
	if err := svc.UpdateAgentDef("proj1", wfID, "upd-main", &types.AgentDefUpdateRequest{
		LowConsumptionModel: &lcModel,
	}); err != nil {
		t.Fatalf("UpdateAgentDef: %v", err)
	}

	def, err := svc.GetAgentDef("proj1", wfID, "upd-main")
	if err != nil {
		t.Fatalf("GetAgentDef after update: %v", err)
	}
	if def.LowConsumptionModel != "haiku" {
		t.Errorf("after update LowConsumptionModel = %q, want %q", def.LowConsumptionModel, "haiku")
	}
}

func TestUpdateAgentDef_ClearsLowConsumptionModel(t *testing.T) {
	_, svc, wfID := setupAgentDefTestEnv(t, nil)

	_, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:                  "clr-main",
		Prompt:              "main",
		LowConsumptionModel: "sonnet",
	})
	if err != nil {
		t.Fatalf("create clr-main: %v", err)
	}

	empty := ""
	if err := svc.UpdateAgentDef("proj1", wfID, "clr-main", &types.AgentDefUpdateRequest{
		LowConsumptionModel: &empty,
	}); err != nil {
		t.Fatalf("UpdateAgentDef clear: %v", err)
	}

	def, err := svc.GetAgentDef("proj1", wfID, "clr-main")
	if err != nil {
		t.Fatalf("GetAgentDef after clear: %v", err)
	}
	if def.LowConsumptionModel != "" {
		t.Errorf("after clear LowConsumptionModel = %q, want empty", def.LowConsumptionModel)
	}
}

// --- Validation: invalid model ---

func TestCreateAgentDef_InvalidLowConsumptionModel(t *testing.T) {
	_, svc, wfID := setupAgentDefTestEnv(t, nil)

	_, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:                  "bad-model",
		Prompt:              "bad",
		LowConsumptionModel: "invalid_model",
	})
	if err == nil {
		t.Fatal("expected error for invalid low_consumption_model, got nil")
	}
}

func TestUpdateAgentDef_InvalidLowConsumptionModel(t *testing.T) {
	_, svc, wfID := setupAgentDefTestEnv(t, nil)

	_, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:     "upd-bad",
		Prompt: "bad",
	})
	if err != nil {
		t.Fatalf("create upd-bad: %v", err)
	}

	invalid := "not_a_model"
	if err := svc.UpdateAgentDef("proj1", wfID, "upd-bad", &types.AgentDefUpdateRequest{
		LowConsumptionModel: &invalid,
	}); err == nil {
		t.Fatal("expected error for invalid low_consumption_model update, got nil")
	}
}

// --- Casing: value is lowercased on create and update ---

func TestCreateAgentDef_LowConsumptionModelLowercased(t *testing.T) {
	_, svc, wfID := setupAgentDefTestEnv(t, nil)

	def, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:                  "lc-lower-main",
		Prompt:              "main",
		LowConsumptionModel: "SONNET", // upper case
	})
	if err != nil {
		t.Fatalf("CreateAgentDef with mixed-case low_consumption_model: %v", err)
	}
	if def.LowConsumptionModel != "sonnet" {
		t.Errorf("LowConsumptionModel = %q, want %q", def.LowConsumptionModel, "sonnet")
	}
}

func TestUpdateAgentDef_LowConsumptionModelLowercased(t *testing.T) {
	_, svc, wfID := setupAgentDefTestEnv(t, nil)

	_, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{ID: "case-main", Prompt: "main"})
	if err != nil {
		t.Fatalf("create case-main: %v", err)
	}

	mixed := "HAIKU"
	if err := svc.UpdateAgentDef("proj1", wfID, "case-main", &types.AgentDefUpdateRequest{
		LowConsumptionModel: &mixed,
	}); err != nil {
		t.Fatalf("UpdateAgentDef: %v", err)
	}

	def, err := svc.GetAgentDef("proj1", wfID, "case-main")
	if err != nil {
		t.Fatalf("GetAgentDef after update: %v", err)
	}
	if def.LowConsumptionModel != "haiku" {
		t.Errorf("after update LowConsumptionModel = %q, want %q", def.LowConsumptionModel, "haiku")
	}
}

// --- Default: empty low_consumption_model ---

func TestCreateAgentDef_DefaultEmptyLowConsumptionModel(t *testing.T) {
	_, svc, wfID := setupAgentDefTestEnv(t, nil)

	def, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:     "no-lc",
		Prompt: "no lc",
	})
	if err != nil {
		t.Fatalf("CreateAgentDef: %v", err)
	}
	if def.LowConsumptionModel != "" {
		t.Errorf("LowConsumptionModel = %q, want empty", def.LowConsumptionModel)
	}

	got, err := svc.GetAgentDef("proj1", wfID, "no-lc")
	if err != nil {
		t.Fatalf("GetAgentDef: %v", err)
	}
	if got.LowConsumptionModel != "" {
		t.Errorf("GetAgentDef LowConsumptionModel = %q, want empty", got.LowConsumptionModel)
	}
}

