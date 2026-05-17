package service

import (
	"errors"
	"testing"

	"be/internal/clock"
	"be/internal/types"
)

// setupAgentDefAPIModeEnv returns a single service and a settings service sharing the
// same pool. The pool's api_mode_enabled setting starts unset (off).
func setupAgentDefAPIModeEnv(t *testing.T) (svc *AgentDefinitionService, settingsSvc *GlobalSettingsService, wfID string) {
	t.Helper()
	pool, _, wfID := setupAgentDefTestEnv(t, nil)
	cliModelSvc := NewCLIModelService(pool, clock.Real())
	svc = NewAgentDefinitionService(pool, clock.Real(), cliModelSvc, nil)
	settingsSvc = NewGlobalSettingsService(pool, clock.Real())
	return svc, settingsSvc, wfID
}

// TestCreateAgentDef_ErrAPIModeDisabled verifies that creating an agent definition with
// execution_mode="api" returns ErrAPIModeDisabled when the setting is not enabled.
func TestCreateAgentDef_ErrAPIModeDisabled(t *testing.T) {
	t.Parallel()
	svc, _, wfID := setupAgentDefAPIModeEnv(t)

	_, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:            "agent-api",
		Prompt:        "do stuff",
		ExecutionMode: "api",
	})
	if err == nil {
		t.Fatal("CreateAgentDef with execution_mode=api (setting off): expected error, got nil")
	}
	if !errors.Is(err, ErrAPIModeDisabled) {
		t.Errorf("CreateAgentDef error = %v, want ErrAPIModeDisabled", err)
	}
}

// TestCreateAgentDef_APIMode_Succeeds verifies that creating an agent definition with
// execution_mode="api" succeeds after setting api_mode_enabled=true.
func TestCreateAgentDef_APIMode_Succeeds(t *testing.T) {
	t.Parallel()
	svc, settingsSvc, wfID := setupAgentDefAPIModeEnv(t)
	if err := settingsSvc.Set("api_mode_enabled", "true"); err != nil {
		t.Fatalf("Set api_mode_enabled: %v", err)
	}

	def, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:            "agent-api-on",
		Prompt:        "do stuff",
		ExecutionMode: "api",
	})
	if err != nil {
		t.Fatalf("CreateAgentDef with execution_mode=api (setting on): %v", err)
	}
	if def.ExecutionMode != "api" {
		t.Errorf("ExecutionMode = %q, want %q", def.ExecutionMode, "api")
	}
}

// TestCreateAgentDef_CLIInteractiveMode_SucceedsWhenAPIModeOff verifies that
// execution_mode="cli_interactive" agents are always accepted regardless of the setting.
func TestCreateAgentDef_CLIInteractiveMode_SucceedsWhenAPIModeOff(t *testing.T) {
	t.Parallel()
	svc, _, wfID := setupAgentDefAPIModeEnv(t)

	def, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:            "agent-cli",
		Prompt:        "do stuff",
		ExecutionMode: "cli_interactive",
	})
	if err != nil {
		t.Fatalf("CreateAgentDef with execution_mode=cli_interactive (setting off): %v", err)
	}
	if def.ExecutionMode != "cli_interactive" {
		t.Errorf("ExecutionMode = %q, want %q", def.ExecutionMode, "cli_interactive")
	}
}

// TestCreateAgentDef_DefaultExecutionMode verifies that omitting execution_mode
// defaults to "cli_interactive" and succeeds regardless of setting.
func TestCreateAgentDef_DefaultExecutionMode(t *testing.T) {
	t.Parallel()
	svc, _, wfID := setupAgentDefAPIModeEnv(t)

	def, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:     "agent-default",
		Prompt: "do stuff",
		// ExecutionMode intentionally omitted
	})
	if err != nil {
		t.Fatalf("CreateAgentDef with default execution_mode: %v", err)
	}
	if def.ExecutionMode != "cli_interactive" {
		t.Errorf("default ExecutionMode = %q, want %q", def.ExecutionMode, "cli_interactive")
	}
}

// TestUpdateAgentDef_ErrAPIModeDisabled verifies that updating execution_mode to "api"
// returns ErrAPIModeDisabled when the setting is not enabled.
func TestUpdateAgentDef_ErrAPIModeDisabled(t *testing.T) {
	t.Parallel()
	svc, _, wfID := setupAgentDefAPIModeEnv(t)

	// Create cli_interactive agent (no api mode needed).
	_, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:            "upd-to-api",
		Prompt:        "do stuff",
		ExecutionMode: "cli_interactive",
	})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}

	mode := "api"
	err = svc.UpdateAgentDef("proj1", wfID, "upd-to-api", &types.AgentDefUpdateRequest{
		ExecutionMode: &mode,
	})
	if err == nil {
		t.Fatal("UpdateAgentDef to execution_mode=api (setting off): expected error, got nil")
	}
	if !errors.Is(err, ErrAPIModeDisabled) {
		t.Errorf("UpdateAgentDef error = %v, want ErrAPIModeDisabled", err)
	}
}

// TestUpdateAgentDef_APIMode_Succeeds verifies that updating execution_mode to "api"
// succeeds after enabling the setting.
func TestUpdateAgentDef_APIMode_Succeeds(t *testing.T) {
	t.Parallel()
	svc, settingsSvc, wfID := setupAgentDefAPIModeEnv(t)

	_, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:            "upd-to-api-on",
		Prompt:        "do stuff",
		ExecutionMode: "cli_interactive",
	})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}

	if err := settingsSvc.Set("api_mode_enabled", "true"); err != nil {
		t.Fatalf("Set api_mode_enabled: %v", err)
	}

	mode := "api"
	if err := svc.UpdateAgentDef("proj1", wfID, "upd-to-api-on", &types.AgentDefUpdateRequest{
		ExecutionMode: &mode,
	}); err != nil {
		t.Fatalf("UpdateAgentDef to execution_mode=api (setting on): %v", err)
	}

	def, err := svc.GetAgentDef("proj1", wfID, "upd-to-api-on")
	if err != nil {
		t.Fatalf("GetAgentDef: %v", err)
	}
	if def.ExecutionMode != "api" {
		t.Errorf("after update ExecutionMode = %q, want %q", def.ExecutionMode, "api")
	}
}
