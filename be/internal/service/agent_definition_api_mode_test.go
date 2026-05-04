package service

import (
	"errors"
	"testing"

	"be/internal/clock"
	"be/internal/types"
)

// setupAgentDefAPIModeEnv returns two services sharing the same pool:
// svcOff has apiMode=false, svcOn has apiMode=true.
func setupAgentDefAPIModeEnv(t *testing.T) (svcOff, svcOn *AgentDefinitionService, wfID string) {
	t.Helper()
	pool, _, wfID := setupAgentDefTestEnv(t, nil)
	cliModelSvc := NewCLIModelService(pool, clock.Real())
	svcOff = NewAgentDefinitionService(pool, clock.Real(), cliModelSvc, false)
	svcOn = NewAgentDefinitionService(pool, clock.Real(), cliModelSvc, true)
	return svcOff, svcOn, wfID
}

// TestCreateAgentDef_ErrAPIModeDisabled verifies that creating an agent definition with
// execution_mode="api" returns ErrAPIModeDisabled when the service was constructed with apiMode=false.
func TestCreateAgentDef_ErrAPIModeDisabled(t *testing.T) {
	t.Parallel()
	svcOff, _, wfID := setupAgentDefAPIModeEnv(t)

	_, err := svcOff.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:            "agent-api",
		Prompt:        "do stuff",
		ExecutionMode: "api",
	})
	if err == nil {
		t.Fatal("CreateAgentDef with execution_mode=api and apiMode=false: expected error, got nil")
	}
	if !errors.Is(err, ErrAPIModeDisabled) {
		t.Errorf("CreateAgentDef error = %v, want ErrAPIModeDisabled", err)
	}
}

// TestCreateAgentDef_APIMode_Succeeds verifies that creating an agent definition with
// execution_mode="api" succeeds when the service was constructed with apiMode=true.
func TestCreateAgentDef_APIMode_Succeeds(t *testing.T) {
	t.Parallel()
	_, svcOn, wfID := setupAgentDefAPIModeEnv(t)

	def, err := svcOn.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:            "agent-api-on",
		Prompt:        "do stuff",
		ExecutionMode: "api",
	})
	if err != nil {
		t.Fatalf("CreateAgentDef with execution_mode=api and apiMode=true: %v", err)
	}
	if def.ExecutionMode != "api" {
		t.Errorf("ExecutionMode = %q, want %q", def.ExecutionMode, "api")
	}
}

// TestCreateAgentDef_CLIMode_SucceedsWhenAPIModeOff verifies that execution_mode="cli"
// agents are always accepted regardless of the service apiMode flag.
func TestCreateAgentDef_CLIMode_SucceedsWhenAPIModeOff(t *testing.T) {
	t.Parallel()
	svcOff, _, wfID := setupAgentDefAPIModeEnv(t)

	def, err := svcOff.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:            "agent-cli",
		Prompt:        "do stuff",
		ExecutionMode: "cli",
	})
	if err != nil {
		t.Fatalf("CreateAgentDef with execution_mode=cli and apiMode=false: %v", err)
	}
	if def.ExecutionMode != "cli" {
		t.Errorf("ExecutionMode = %q, want %q", def.ExecutionMode, "cli")
	}
}

// TestCreateAgentDef_DefaultExecutionMode verifies that omitting execution_mode
// defaults to "cli" and succeeds regardless of apiMode.
func TestCreateAgentDef_DefaultExecutionMode(t *testing.T) {
	t.Parallel()
	svcOff, _, wfID := setupAgentDefAPIModeEnv(t)

	def, err := svcOff.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:     "agent-default",
		Prompt: "do stuff",
		// ExecutionMode intentionally omitted
	})
	if err != nil {
		t.Fatalf("CreateAgentDef with default execution_mode and apiMode=false: %v", err)
	}
	if def.ExecutionMode != "cli" {
		t.Errorf("default ExecutionMode = %q, want %q", def.ExecutionMode, "cli")
	}
}

// TestUpdateAgentDef_ErrAPIModeDisabled verifies that updating execution_mode to "api"
// returns ErrAPIModeDisabled when the service was constructed with apiMode=false.
func TestUpdateAgentDef_ErrAPIModeDisabled(t *testing.T) {
	t.Parallel()
	svcOff, svcOn, wfID := setupAgentDefAPIModeEnv(t)

	// Create a CLI agent using the apiMode=true service so it succeeds
	_, err := svcOn.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:            "upd-to-api",
		Prompt:        "do stuff",
		ExecutionMode: "cli",
	})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}

	apiMode := "api"
	err = svcOff.UpdateAgentDef("proj1", wfID, "upd-to-api", &types.AgentDefUpdateRequest{
		ExecutionMode: &apiMode,
	})
	if err == nil {
		t.Fatal("UpdateAgentDef to execution_mode=api with apiMode=false: expected error, got nil")
	}
	if !errors.Is(err, ErrAPIModeDisabled) {
		t.Errorf("UpdateAgentDef error = %v, want ErrAPIModeDisabled", err)
	}
}

// TestUpdateAgentDef_APIMode_Succeeds verifies that updating execution_mode to "api"
// succeeds when the service was constructed with apiMode=true.
func TestUpdateAgentDef_APIMode_Succeeds(t *testing.T) {
	t.Parallel()
	_, svcOn, wfID := setupAgentDefAPIModeEnv(t)

	_, err := svcOn.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:            "upd-to-api-on",
		Prompt:        "do stuff",
		ExecutionMode: "cli",
	})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}

	apiMode := "api"
	if err := svcOn.UpdateAgentDef("proj1", wfID, "upd-to-api-on", &types.AgentDefUpdateRequest{
		ExecutionMode: &apiMode,
	}); err != nil {
		t.Fatalf("UpdateAgentDef to execution_mode=api with apiMode=true: %v", err)
	}

	def, err := svcOn.GetAgentDef("proj1", wfID, "upd-to-api-on")
	if err != nil {
		t.Fatalf("GetAgentDef: %v", err)
	}
	if def.ExecutionMode != "api" {
		t.Errorf("after update ExecutionMode = %q, want %q", def.ExecutionMode, "api")
	}
}
