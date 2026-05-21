package service

import (
	"testing"

	"be/internal/types"
)

// setupConsultantEnv returns a service and settings service with api_mode_enabled=true.
func setupConsultantEnv(t *testing.T) (svc *AgentDefinitionService, settingsSvc *GlobalSettingsService, wfID string) {
	t.Helper()
	svc, settingsSvc, wfID = setupAgentDefAPIModeEnv(t)
	if err := settingsSvc.Set("api_mode_enabled", "true"); err != nil {
		t.Fatalf("Set api_mode_enabled: %v", err)
	}
	return svc, settingsSvc, wfID
}

// TestCreateConsultant_APIMode_RoundTrip verifies that consultant=true with
// execution_mode=api is accepted and the flag persists through Get and List.
func TestCreateConsultant_APIMode_RoundTrip(t *testing.T) {
	t.Parallel()
	svc, _, wfID := setupConsultantEnv(t)

	def, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:            "consultant-a",
		Prompt:        "advise",
		ExecutionMode: "api",
		Consultant:    true,
	})
	if err != nil {
		t.Fatalf("CreateAgentDef(consultant=true, api): %v", err)
	}
	if !def.Consultant {
		t.Error("Create returned Consultant=false, want true")
	}
	if def.ExecutionMode != "api" {
		t.Errorf("Create ExecutionMode = %q, want api", def.ExecutionMode)
	}

	got, err := svc.GetAgentDef("proj1", wfID, "consultant-a")
	if err != nil {
		t.Fatalf("GetAgentDef: %v", err)
	}
	if !got.Consultant {
		t.Error("GetAgentDef Consultant = false, want true")
	}

	list, err := svc.ListAgentDefs("proj1", wfID)
	if err != nil {
		t.Fatalf("ListAgentDefs: %v", err)
	}
	var found bool
	for _, d := range list {
		if d.ID == "consultant-a" {
			found = true
			if !d.Consultant {
				t.Error("ListAgentDefs Consultant = false, want true")
			}
		}
	}
	if !found {
		t.Error("agent consultant-a not in ListAgentDefs result")
	}
}

// TestCreateConsultant_NonAPI_Rejected verifies that consultant=true is rejected
// for cli_interactive and script execution modes.
func TestCreateConsultant_NonAPI_Rejected(t *testing.T) {
	t.Parallel()
	svc, _, wfID := setupConsultantEnv(t)

	cases := []string{"cli_interactive", "script"}
	for _, mode := range cases {
		mode := mode
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			req := &types.AgentDefCreateRequest{
				ID:            "cons-" + mode,
				Prompt:        "advise",
				ExecutionMode: mode,
				Consultant:    true,
			}
			_, err := svc.CreateAgentDef("proj1", wfID, req)
			if err == nil {
				t.Errorf("CreateAgentDef(consultant=true, %s): expected error, got nil", mode)
			}
		})
	}
}

// TestCreateConsultant_False_NonAPI_Succeeds verifies consultant=false (the default)
// is accepted for all non-api execution modes.
func TestCreateConsultant_False_NonAPI_Succeeds(t *testing.T) {
	t.Parallel()
	svc, _, wfID := setupConsultantEnv(t)

	def, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:            "non-consultant",
		Prompt:        "do stuff",
		ExecutionMode: "cli_interactive",
		Consultant:    false,
	})
	if err != nil {
		t.Fatalf("CreateAgentDef(consultant=false, cli_interactive): %v", err)
	}
	if def.Consultant {
		t.Error("Create returned Consultant=true, want false")
	}
}

// TestUpdateConsultant_FlipModeAndClearConsultant_Succeeds verifies that
// simultaneously setting execution_mode=cli_interactive and consultant=false on
// a consultant agent is accepted (both fields change together).
func TestUpdateConsultant_FlipModeAndClearConsultant_Succeeds(t *testing.T) {
	t.Parallel()
	svc, _, wfID := setupConsultantEnv(t)

	if _, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:            "flip-mode-clear",
		Prompt:        "advise",
		ExecutionMode: "api",
		Consultant:    true,
	}); err != nil {
		t.Fatalf("create consultant agent: %v", err)
	}

	cliMode := "cli_interactive"
	noConsultant := false
	if err := svc.UpdateAgentDef("proj1", wfID, "flip-mode-clear", &types.AgentDefUpdateRequest{
		ExecutionMode: &cliMode,
		Consultant:    &noConsultant,
	}); err != nil {
		t.Fatalf("UpdateAgentDef(mode=cli_interactive, consultant=false): %v", err)
	}

	def, err := svc.GetAgentDef("proj1", wfID, "flip-mode-clear")
	if err != nil {
		t.Fatalf("GetAgentDef after update: %v", err)
	}
	if def.Consultant {
		t.Error("after update Consultant = true, want false")
	}
	if def.ExecutionMode != "cli_interactive" {
		t.Errorf("after update ExecutionMode = %q, want cli_interactive", def.ExecutionMode)
	}
}

// TestUpdateConsultant_SetTrueOnNonAPI_Rejected verifies that flipping consultant=true
// on an agent whose current execution_mode is not "api" is rejected.
func TestUpdateConsultant_SetTrueOnNonAPI_Rejected(t *testing.T) {
	t.Parallel()
	svc, _, wfID := setupConsultantEnv(t)

	if _, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:            "non-api-agent",
		Prompt:        "do stuff",
		ExecutionMode: "cli_interactive",
	}); err != nil {
		t.Fatalf("create cli_interactive agent: %v", err)
	}

	consultant := true
	err := svc.UpdateAgentDef("proj1", wfID, "non-api-agent", &types.AgentDefUpdateRequest{
		Consultant: &consultant,
	})
	if err == nil {
		t.Fatal("UpdateAgentDef(consultant=true on cli_interactive): expected error, got nil")
	}
}

// TestUpdateConsultant_FlipModeOnly_Rejected verifies that flipping
// execution_mode away from "api" on an existing consultant agent is rejected
// even when the request does not include the consultant field.
func TestUpdateConsultant_FlipModeOnly_Rejected(t *testing.T) {
	t.Parallel()
	svc, _, wfID := setupConsultantEnv(t)

	if _, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:            "flip-mode-only",
		Prompt:        "advise",
		ExecutionMode: "api",
		Consultant:    true,
	}); err != nil {
		t.Fatalf("create consultant agent: %v", err)
	}

	cliMode := "cli_interactive"
	err := svc.UpdateAgentDef("proj1", wfID, "flip-mode-only", &types.AgentDefUpdateRequest{
		ExecutionMode: &cliMode,
	})
	if err == nil {
		t.Fatal("UpdateAgentDef(execution_mode=cli_interactive on consultant): expected error, got nil")
	}

	// Ensure the rejected update did not mutate the row.
	def, getErr := svc.GetAgentDef("proj1", wfID, "flip-mode-only")
	if getErr != nil {
		t.Fatalf("GetAgentDef after rejected update: %v", getErr)
	}
	if def.ExecutionMode != "api" {
		t.Errorf("after rejected update ExecutionMode = %q, want api", def.ExecutionMode)
	}
	if !def.Consultant {
		t.Error("after rejected update Consultant = false, want true")
	}
}

// TestUpdateConsultant_AndModeToAPI_Succeeds verifies that updating both
// consultant=true and execution_mode=api in a single request is accepted.
func TestUpdateConsultant_AndModeToAPI_Succeeds(t *testing.T) {
	t.Parallel()
	svc, _, wfID := setupConsultantEnv(t)

	if _, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:            "upgrade-to-consultant",
		Prompt:        "do stuff",
		ExecutionMode: "cli_interactive",
	}); err != nil {
		t.Fatalf("create cli_interactive agent: %v", err)
	}

	apiMode := "api"
	consultant := true
	if err := svc.UpdateAgentDef("proj1", wfID, "upgrade-to-consultant", &types.AgentDefUpdateRequest{
		ExecutionMode: &apiMode,
		Consultant:    &consultant,
	}); err != nil {
		t.Fatalf("UpdateAgentDef(consultant=true, execution_mode=api): %v", err)
	}

	def, err := svc.GetAgentDef("proj1", wfID, "upgrade-to-consultant")
	if err != nil {
		t.Fatalf("GetAgentDef after update: %v", err)
	}
	if !def.Consultant {
		t.Error("after update Consultant = false, want true")
	}
	if def.ExecutionMode != "api" {
		t.Errorf("after update ExecutionMode = %q, want api", def.ExecutionMode)
	}
}

// TestUpdateConsultant_ClearFlag_Succeeds verifies consultant=false on an api
// consultant agent is accepted.
func TestUpdateConsultant_ClearFlag_Succeeds(t *testing.T) {
	t.Parallel()
	svc, _, wfID := setupConsultantEnv(t)

	if _, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:            "clear-consultant",
		Prompt:        "advise",
		ExecutionMode: "api",
		Consultant:    true,
	}); err != nil {
		t.Fatalf("create consultant agent: %v", err)
	}

	noConsultant := false
	if err := svc.UpdateAgentDef("proj1", wfID, "clear-consultant", &types.AgentDefUpdateRequest{
		Consultant: &noConsultant,
	}); err != nil {
		t.Fatalf("UpdateAgentDef(consultant=false): %v", err)
	}

	def, err := svc.GetAgentDef("proj1", wfID, "clear-consultant")
	if err != nil {
		t.Fatalf("GetAgentDef after clear: %v", err)
	}
	if def.Consultant {
		t.Error("after clear Consultant = true, want false")
	}
}
