package service

import (
	"fmt"
	"testing"

	"be/internal/types"
)

// --- Table-driven: all valid model names accepted, invalid rejected ---

func TestCreateAgentDef_LowConsumptionModel_ValidModels(t *testing.T) {
	validModels := []string{
		"opus", "opus_1m", "sonnet", "haiku",
		"opencode_minimax_m25_free", "opencode_qwen36_plus_free", "opencode_gpt54",
		"codex_gpt_normal", "codex_gpt_high",
		"codex_gpt54_normal", "codex_gpt54_high",
	}

	for i, m := range validModels {
		t.Run(m, func(t *testing.T) {
			_, svc, wfID := setupAgentDefTestEnv(t, nil)
			id := fmt.Sprintf("vm-%d", i)
			def, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
				ID:                  id,
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
	invalidModels := []string{
		"invalid_model", "gpt-4", "claude-3", "lite-implementor",
		"opus3", "sonnet2", "unknown",
	}

	for _, m := range invalidModels {
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

func TestUpdateAgentDef_LowConsumptionModel_InvalidModels(t *testing.T) {
	_, svc, wfID := setupAgentDefTestEnv(t, nil)

	// Create a base agent without low_consumption_model
	_, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:     "upd-inv-lcm",
		Prompt: "test",
	})
	if err != nil {
		t.Fatalf("create base agent: %v", err)
	}

	invalidModels := []string{
		"invalid_model", "gpt-4", "claude-3", "lite-implementor",
		"opus3", "sonnet2", "unknown",
	}

	for _, m := range invalidModels {
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

func TestUpdateAgentDef_LowConsumptionModel_ValidModels(t *testing.T) {
	validModels := []string{
		"opus", "opus_1m", "haiku", "sonnet",
		"opencode_minimax_m25_free", "opencode_qwen36_plus_free", "opencode_gpt54",
		"codex_gpt_normal", "codex_gpt_high",
		"codex_gpt54_normal", "codex_gpt54_high",
	}

	_, svc, wfID := setupAgentDefTestEnv(t, nil)

	// Create one agent per model and update it to that valid model
	for i, m := range validModels {
		id := fmt.Sprintf("upd-vm-%d", i)
		_, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{ID: id, Prompt: "p"})
		if err != nil {
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
