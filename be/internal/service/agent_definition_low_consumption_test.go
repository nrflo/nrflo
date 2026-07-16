package service

import (
	"fmt"
	"testing"

	"be/internal/types"
)

// lcValidModels is the set of low_consumption_model values accepted by validation.
var lcValidModels = []string{
	"fable-5", "opus-4-6", "opus-4-6-1m", "opus-4-7", "opus-4-7-1m", "sonnet-5", "haiku-4-5",
	"gpt-5.4", "gpt-5.4-mini", "gpt-5.5",
}

// lcInvalidModels is a set of values that must be rejected by validation.
var lcInvalidModels = []string{
	"invalid_model", "gpt-4", "claude-3", "lite-implementor",
	"opus3", "sonnet2", "unknown",
}

// --- Validation: every valid model accepted, every invalid model rejected ---

func TestCreateAgentDef_LowConsumptionModel_ValidModels(t *testing.T) {
	t.Parallel()
	for i, m := range lcValidModels {
		t.Run(m, func(t *testing.T) {
			_, svc, wfID := setupAgentDefTestEnv(t, nil)
			def, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
				ID:                  fmt.Sprintf("vm-%d", i),
				Prompt:              "p",
				LowConsumptionModel: m,
			})
			if err != nil {
				t.Fatalf("CreateAgentDef(low_consumption_model=%q) error = %v, want nil", m, err)
			}
			if def.LowConsumptionModel != m {
				t.Errorf("LowConsumptionModel = %q, want %q", def.LowConsumptionModel, m)
			}
		})
	}
}

func TestCreateAgentDef_LowConsumptionModel_InvalidModels(t *testing.T) {
	t.Parallel()
	for _, m := range lcInvalidModels {
		t.Run(m, func(t *testing.T) {
			_, svc, wfID := setupAgentDefTestEnv(t, nil)
			_, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
				ID:                  "inv-" + m,
				Prompt:              "p",
				LowConsumptionModel: m,
			})
			if err == nil {
				t.Errorf("CreateAgentDef(low_consumption_model=%q) error = nil, want error", m)
			}
		})
	}
}

func TestUpdateAgentDef_LowConsumptionModel_ValidModels(t *testing.T) {
	t.Parallel()
	_, svc, wfID := setupAgentDefTestEnv(t, nil)

	for i, m := range lcValidModels {
		id := fmt.Sprintf("upd-vm-%d", i)
		if _, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{ID: id, Prompt: "p"}); err != nil {
			t.Fatalf("create %s: %v", id, err)
		}
		t.Run(m, func(t *testing.T) {
			lcm := m
			if err := svc.UpdateAgentDef("proj1", wfID, id, &types.AgentDefUpdateRequest{
				LowConsumptionModel: &lcm,
			}); err != nil {
				t.Fatalf("UpdateAgentDef(low_consumption_model=%q) error = %v, want nil", lcm, err)
			}
			def, err := svc.GetAgentDef("proj1", wfID, id)
			if err != nil {
				t.Fatalf("GetAgentDef: %v", err)
			}
			if def.LowConsumptionModel != lcm {
				t.Errorf("LowConsumptionModel = %q, want %q", def.LowConsumptionModel, lcm)
			}
		})
	}
}

func TestUpdateAgentDef_LowConsumptionModel_InvalidModels(t *testing.T) {
	t.Parallel()
	_, svc, wfID := setupAgentDefTestEnv(t, nil)

	if _, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID: "upd-inv-lcm", Prompt: "test",
	}); err != nil {
		t.Fatalf("create base agent: %v", err)
	}

	for _, m := range lcInvalidModels {
		t.Run(m, func(t *testing.T) {
			lcm := m
			if err := svc.UpdateAgentDef("proj1", wfID, "upd-inv-lcm", &types.AgentDefUpdateRequest{
				LowConsumptionModel: &lcm,
			}); err == nil {
				t.Errorf("UpdateAgentDef(low_consumption_model=%q) = nil, want error", lcm)
			}
		})
	}
}

// --- CRUD round-trips ---

func TestGetAgentDef_ReturnsLowConsumptionModel(t *testing.T) {
	t.Parallel()
	_, svc, wfID := setupAgentDefTestEnv(t, nil)

	if _, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID: "main-agent", Prompt: "main", LowConsumptionModel: "haiku-4-5",
	}); err != nil {
		t.Fatalf("create main-agent: %v", err)
	}

	got, err := svc.GetAgentDef("proj1", wfID, "main-agent")
	if err != nil {
		t.Fatalf("GetAgentDef: %v", err)
	}
	if got.LowConsumptionModel != "haiku-4-5" {
		t.Errorf("GetAgentDef LowConsumptionModel = %q, want %q", got.LowConsumptionModel, "haiku-4-5")
	}
}

func TestListAgentDefs_ReturnsLowConsumptionModel(t *testing.T) {
	t.Parallel()
	_, svc, wfID := setupAgentDefTestEnv(t, nil)

	if _, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{ID: "la-ref", Prompt: "ref"}); err != nil {
		t.Fatalf("create la-ref: %v", err)
	}
	if _, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID: "la-main", Prompt: "main", LowConsumptionModel: "sonnet-5",
	}); err != nil {
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
	if defs[0].LowConsumptionModel != "sonnet-5" {
		t.Errorf("ListAgentDefs[0].LowConsumptionModel = %q, want %q", defs[0].LowConsumptionModel, "sonnet-5")
	}
	if defs[1].LowConsumptionModel != "" {
		t.Errorf("ListAgentDefs[1].LowConsumptionModel = %q, want empty", defs[1].LowConsumptionModel)
	}
}

func TestUpdateAgentDef_ClearsLowConsumptionModel(t *testing.T) {
	t.Parallel()
	_, svc, wfID := setupAgentDefTestEnv(t, nil)

	if _, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID: "clr-main", Prompt: "main", LowConsumptionModel: "sonnet-5",
	}); err != nil {
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

func TestCreateAgentDef_DefaultEmptyLowConsumptionModel(t *testing.T) {
	t.Parallel()
	_, svc, wfID := setupAgentDefTestEnv(t, nil)

	def, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID: "no-lc", Prompt: "no lc",
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
