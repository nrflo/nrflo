package service

import (
	"errors"
	"testing"

	"be/internal/clock"
	"be/internal/types"
)

// setupLiveSettingEnv creates a single AgentDefinitionService and GlobalSettingsService
// sharing the same pool. api_mode_enabled starts unset (off).
func setupLiveSettingEnv(t *testing.T) (*AgentDefinitionService, *GlobalSettingsService, string) {
	t.Helper()
	pool, _, wfID := setupAgentDefTestEnv(t, nil)
	cliModelSvc := NewCLIModelService(pool, clock.Real())
	svc := NewAgentDefinitionService(pool, clock.Real(), cliModelSvc, nil)
	settingsSvc := NewGlobalSettingsService(pool, clock.Real())
	return svc, settingsSvc, wfID
}

// TestLiveSetting_Create_DisabledThenEnabled verifies that Create with execution_mode=api
// fails when the setting is off, then succeeds immediately after enabling it — no restart
// or new service constructor required.
func TestLiveSetting_Create_DisabledThenEnabled(t *testing.T) {
	svc, settingsSvc, wfID := setupLiveSettingEnv(t)

	// Create fails when setting is absent.
	_, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:            "live-api-agent",
		Prompt:        "do stuff",
		ExecutionMode: "api",
	})
	if err == nil {
		t.Fatal("expected ErrAPIModeDisabled, got nil")
	}
	if !errors.Is(err, ErrAPIModeDisabled) {
		t.Fatalf("Create (setting off) error = %v, want ErrAPIModeDisabled", err)
	}

	// Enable setting.
	if err := settingsSvc.Set("api_mode_enabled", "true"); err != nil {
		t.Fatalf("Set api_mode_enabled=true: %v", err)
	}

	// Same service instance — no reconstruction — succeeds now.
	def, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:            "live-api-agent",
		Prompt:        "do stuff",
		ExecutionMode: "api",
	})
	if err != nil {
		t.Fatalf("Create (setting on) error = %v, want nil", err)
	}
	if def.ExecutionMode != "api" {
		t.Errorf("ExecutionMode = %q, want api", def.ExecutionMode)
	}
}

// TestLiveSetting_Create_EnabledThenDisabled verifies that disabling the setting
// after creation gates subsequent api-mode creates immediately.
func TestLiveSetting_Create_EnabledThenDisabled(t *testing.T) {
	svc, settingsSvc, wfID := setupLiveSettingEnv(t)

	// Enable setting first.
	if err := settingsSvc.Set("api_mode_enabled", "true"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	// First create succeeds.
	if _, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID: "api-agent-1", Prompt: "do stuff", ExecutionMode: "api",
	}); err != nil {
		t.Fatalf("Create (enabled) = %v, want nil", err)
	}

	// Disable setting.
	if err := settingsSvc.Set("api_mode_enabled", "false"); err != nil {
		t.Fatalf("Set false: %v", err)
	}

	// Second create is blocked.
	_, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID: "api-agent-2", Prompt: "do stuff", ExecutionMode: "api",
	})
	if err == nil {
		t.Fatal("expected ErrAPIModeDisabled after disable, got nil")
	}
	if !errors.Is(err, ErrAPIModeDisabled) {
		t.Fatalf("Create (after disable) error = %v, want ErrAPIModeDisabled", err)
	}
}

// TestLiveSetting_Update_DisabledThenEnabled verifies that Update with execution_mode=api
// fails when setting is off, then succeeds after enabling.
func TestLiveSetting_Update_DisabledThenEnabled(t *testing.T) {
	svc, settingsSvc, wfID := setupLiveSettingEnv(t)

	// Create cli_interactive agent (no api mode needed).
	if _, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID: "update-target", Prompt: "do stuff", ExecutionMode: "cli_interactive",
	}); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	mode := "api"

	// Update to api fails when setting is off.
	err := svc.UpdateAgentDef("proj1", wfID, "update-target", &types.AgentDefUpdateRequest{
		ExecutionMode: &mode,
	})
	if err == nil {
		t.Fatal("expected ErrAPIModeDisabled, got nil")
	}
	if !errors.Is(err, ErrAPIModeDisabled) {
		t.Fatalf("Update (setting off) error = %v, want ErrAPIModeDisabled", err)
	}

	// Enable setting.
	if err := settingsSvc.Set("api_mode_enabled", "true"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	// Same service instance — update now succeeds.
	if err := svc.UpdateAgentDef("proj1", wfID, "update-target", &types.AgentDefUpdateRequest{
		ExecutionMode: &mode,
	}); err != nil {
		t.Fatalf("Update (setting on) error = %v, want nil", err)
	}

	def, err := svc.GetAgentDef("proj1", wfID, "update-target")
	if err != nil {
		t.Fatalf("GetAgentDef: %v", err)
	}
	if def.ExecutionMode != "api" {
		t.Errorf("ExecutionMode = %q, want api", def.ExecutionMode)
	}
}

// TestLiveSetting_Update_EnabledThenDisabled verifies that disabling the setting
// gates subsequent api-mode updates immediately on the same service instance.
func TestLiveSetting_Update_EnabledThenDisabled(t *testing.T) {
	svc, settingsSvc, wfID := setupLiveSettingEnv(t)

	if err := settingsSvc.Set("api_mode_enabled", "true"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	// Create cli_interactive agent.
	if _, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID: "toggle-target", Prompt: "do stuff", ExecutionMode: "cli_interactive",
	}); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	mode := "api"

	// Update succeeds while enabled.
	if err := svc.UpdateAgentDef("proj1", wfID, "toggle-target", &types.AgentDefUpdateRequest{
		ExecutionMode: &mode,
	}); err != nil {
		t.Fatalf("Update (enabled) = %v, want nil", err)
	}

	// Disable.
	if err := settingsSvc.Set("api_mode_enabled", "false"); err != nil {
		t.Fatalf("Set false: %v", err)
	}

	// Reset back to cli first (using direct DB to bypass the gate).
	cliMode := "cli_interactive"
	if err := svc.UpdateAgentDef("proj1", wfID, "toggle-target", &types.AgentDefUpdateRequest{
		ExecutionMode: &cliMode,
	}); err != nil {
		t.Fatalf("Reset to cli_interactive failed unexpectedly: %v", err)
	}

	// Now re-attempt api update — must fail.
	err := svc.UpdateAgentDef("proj1", wfID, "toggle-target", &types.AgentDefUpdateRequest{
		ExecutionMode: &mode,
	})
	if err == nil {
		t.Fatal("expected ErrAPIModeDisabled after disable, got nil")
	}
	if !errors.Is(err, ErrAPIModeDisabled) {
		t.Fatalf("Update (after disable) error = %v, want ErrAPIModeDisabled", err)
	}
}
