package service

import (
	"strings"
	"testing"

	"be/internal/clock"
	"be/internal/types"
)

// setupAgentDefCLIInteractiveEnv returns a service and workflow ID for cli_interactive tests.
// Delegates to the shared setupAgentDefTestEnv helper (no workflow groups needed).
func setupAgentDefCLIInteractiveEnv(t *testing.T) (*AgentDefinitionService, string) {
	t.Helper()
	_, svc, wfID := setupAgentDefTestEnv(t, nil)
	return svc, wfID
}

// TestCreateAgentDef_CLIInteractive_ClaudeModel verifies that cli_interactive with a claude
// model (opus-4-7) succeeds. No CLIType DB lookup needed — the string-prefix heuristic
// defaults to "claude" for unknown prefixes.
func TestCreateAgentDef_CLIInteractive_ClaudeModel(t *testing.T) {
	t.Parallel()
	svc, wfID := setupAgentDefCLIInteractiveEnv(t)

	def, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:            "agent-cli-int-claude",
		Prompt:        "do stuff",
		ExecutionMode: "cli_interactive",
		Model:         "opus-4-7",
	})
	if err != nil {
		t.Fatalf("CreateAgentDef(cli_interactive, opus-4-7): %v", err)
	}
	if def.ExecutionMode != "cli_interactive" {
		t.Errorf("ExecutionMode = %q, want cli_interactive", def.ExecutionMode)
	}
	if def.Model != "opus-4-7" {
		t.Errorf("Model = %q, want opus-4-7", def.Model)
	}
}

// TestCreateAgentDef_CLIInteractive_CodexModel verifies that cli_interactive with
// an OpenAI registry model succeeds and resolves to the Codex adapter.
func TestCreateAgentDef_CLIInteractive_CodexModel(t *testing.T) {
	t.Parallel()
	svc, wfID := setupAgentDefCLIInteractiveEnv(t)

	def, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:            "agent-cli-int-codex",
		Prompt:        "do stuff",
		ExecutionMode: "cli_interactive",
		Model:         "gpt-5.4",
	})
	if err != nil {
		t.Fatalf("CreateAgentDef(cli_interactive, gpt-5.4): %v", err)
	}
	if def.ExecutionMode != "cli_interactive" {
		t.Errorf("ExecutionMode = %q, want cli_interactive", def.ExecutionMode)
	}
}

// TestCreateAgentDef_CLIInteractive_WithPythonScriptID verifies that cli_interactive with a
// python_script_id is rejected because script IDs require execution_mode="script".
func TestCreateAgentDef_CLIInteractive_WithPythonScriptID(t *testing.T) {
	t.Parallel()
	svc, wfID := setupAgentDefCLIInteractiveEnv(t)

	psID := "ps-xxx"
	_, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:             "agent-cli-int-ps",
		Prompt:         "do stuff",
		ExecutionMode:  "cli_interactive",
		PythonScriptID: &psID,
	})
	if err == nil {
		t.Fatal("CreateAgentDef(cli_interactive + python_script_id): expected error, got nil")
	}
	if !strings.Contains(err.Error(), "python_script_id_requires_script_mode") {
		t.Errorf("error = %v, want python_script_id_requires_script_mode", err)
	}
}

// TestCreateAgentDef_CLIInteractive_DBLookupUsedModel verifies that cli_interactive uses the
// DB-sourced CLIType when the model supports CLI mode. Uses opus-4-7, which is
// seeded with cli_type='claude' in the template DB.
func TestCreateAgentDef_CLIInteractive_DBLookupUsedModel(t *testing.T) {
	t.Parallel()
	pool, _, wfID := setupAgentDefTestEnv(t, nil)
	modelSvc := NewModelService(pool, clock.Real())
	svc := NewAgentDefinitionService(pool, clock.Real(), modelSvc, nil)

	def, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:            "agent-cli-int-db",
		Prompt:        "do stuff",
		ExecutionMode: "cli_interactive",
		Model:         "opus-4-7",
	})
	if err != nil {
		t.Fatalf("CreateAgentDef(cli_interactive, opus-4-7 via DB lookup): %v", err)
	}
	if def.ExecutionMode != "cli_interactive" {
		t.Errorf("ExecutionMode = %q, want cli_interactive", def.ExecutionMode)
	}
}

// TestUpdateAgentDef_ToCLIInteractive_Succeeds verifies that updating execution_mode to
// cli_interactive succeeds when the existing model is compatible (default sonnet-5 → claude).
func TestUpdateAgentDef_ToCLIInteractive_Succeeds(t *testing.T) {
	t.Parallel()
	svc, wfID := setupAgentDefCLIInteractiveEnv(t)

	_, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:     "upd-to-cli-int",
		Prompt: "do stuff",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	mode := "cli_interactive"
	if err := svc.UpdateAgentDef("proj1", wfID, "upd-to-cli-int", &types.AgentDefUpdateRequest{
		ExecutionMode: &mode,
	}); err != nil {
		t.Fatalf("UpdateAgentDef → cli_interactive: %v", err)
	}

	def, err := svc.GetAgentDef("proj1", wfID, "upd-to-cli-int")
	if err != nil {
		t.Fatalf("GetAgentDef: %v", err)
	}
	if def.ExecutionMode != "cli_interactive" {
		t.Errorf("after update ExecutionMode = %q, want cli_interactive", def.ExecutionMode)
	}
}

// TestUpdateAgentDef_ToCLIInteractive_WithNewModel verifies that updating both execution_mode
// to cli_interactive and model to a codex model in one call succeeds.
func TestUpdateAgentDef_ToCLIInteractive_WithNewModel(t *testing.T) {
	t.Parallel()
	svc, wfID := setupAgentDefCLIInteractiveEnv(t)

	_, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:     "upd-mode-and-model",
		Prompt: "do stuff",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	mode := "cli_interactive"
	model := "gpt-5.4"
	if err := svc.UpdateAgentDef("proj1", wfID, "upd-mode-and-model", &types.AgentDefUpdateRequest{
		ExecutionMode: &mode,
		Model:         &model,
	}); err != nil {
		t.Fatalf("UpdateAgentDef → cli_interactive + codex: %v", err)
	}

	def, err := svc.GetAgentDef("proj1", wfID, "upd-mode-and-model")
	if err != nil {
		t.Fatalf("GetAgentDef: %v", err)
	}
	if def.ExecutionMode != "cli_interactive" {
		t.Errorf("ExecutionMode = %q, want cli_interactive", def.ExecutionMode)
	}
	if def.Model != "gpt-5.4" {
		t.Errorf("Model = %q, want gpt-5.4", def.Model)
	}
}

// TestCreateAgentDef_CLIInteractive_ModelValidation exercises all adapter prefix heuristics
// via a table-driven test: known prefixes are accepted, unknown fall back to "claude" and succeed.
func TestCreateAgentDef_CLIInteractive_ModelValidation(t *testing.T) {
	t.Parallel()
	svc, wfID := setupAgentDefCLIInteractiveEnv(t)

	cases := []struct {
		name    string
		agentID string
		model   string
		wantOK  bool
	}{
		{"claude model", "ag-claude", "opus-4-7", true},
		{"codex model", "ag-codex", "gpt-5.4", true},
		{"unknown model rejected", "ag-unknown", "mycompany_model_v1", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
				ID:            tc.agentID,
				Prompt:        "do stuff",
				ExecutionMode: "cli_interactive",
				Model:         tc.model,
			})
			if tc.wantOK && err != nil {
				t.Errorf("CreateAgentDef(cli_interactive, %q): unexpected error: %v", tc.model, err)
			}
			if !tc.wantOK && err == nil {
				t.Errorf("CreateAgentDef(cli_interactive, %q): expected error, got nil", tc.model)
			}
		})
	}
}
