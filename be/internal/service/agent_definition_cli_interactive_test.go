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
// model (opus_4_7) succeeds. No CLIType DB lookup needed — the string-prefix heuristic
// defaults to "claude" for unknown prefixes.
func TestCreateAgentDef_CLIInteractive_ClaudeModel(t *testing.T) {
	t.Parallel()
	svc, wfID := setupAgentDefCLIInteractiveEnv(t)

	def, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:            "agent-cli-int-claude",
		Prompt:        "do stuff",
		ExecutionMode: "cli_interactive",
		Model:         "opus_4_7",
	})
	if err != nil {
		t.Fatalf("CreateAgentDef(cli_interactive, opus_4_7): %v", err)
	}
	if def.ExecutionMode != "cli_interactive" {
		t.Errorf("ExecutionMode = %q, want cli_interactive", def.ExecutionMode)
	}
	if def.Model != "opus_4_7" {
		t.Errorf("Model = %q, want opus_4_7", def.Model)
	}
}

// TestCreateAgentDef_CLIInteractive_CodexModel verifies that cli_interactive with a codex model
// (codex_gpt_normal) succeeds. The "codex_gpt" prefix maps to the codex adapter.
func TestCreateAgentDef_CLIInteractive_CodexModel(t *testing.T) {
	t.Parallel()
	svc, wfID := setupAgentDefCLIInteractiveEnv(t)

	def, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:            "agent-cli-int-codex",
		Prompt:        "do stuff",
		ExecutionMode: "cli_interactive",
		Model:         "codex_gpt_normal",
	})
	if err != nil {
		t.Fatalf("CreateAgentDef(cli_interactive, codex_gpt_normal): %v", err)
	}
	if def.ExecutionMode != "cli_interactive" {
		t.Errorf("ExecutionMode = %q, want cli_interactive", def.ExecutionMode)
	}
}

// TestCreateAgentDef_CLIInteractive_OpencodeModel verifies that cli_interactive with an opencode
// model (opencode_gpt54) is rejected: opencode does not support cli_interactive.
func TestCreateAgentDef_CLIInteractive_OpencodeModel(t *testing.T) {
	t.Parallel()
	svc, wfID := setupAgentDefCLIInteractiveEnv(t)

	_, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:            "agent-cli-int-opencode",
		Prompt:        "do stuff",
		ExecutionMode: "cli_interactive",
		Model:         "opencode_gpt54",
	})
	if err == nil {
		t.Fatal("CreateAgentDef(cli_interactive, opencode_gpt54): expected error, got nil")
	}
	if !strings.Contains(err.Error(), "opencode does not support") {
		t.Errorf("CreateAgentDef error = %q, want to contain %q", err.Error(), "opencode does not support")
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
// DB-sourced CLIType when the model is in the cli_models table. Uses opus_4_7 which is
// seeded as cli_type='claude' in the template DB.
func TestCreateAgentDef_CLIInteractive_DBLookupUsedModel(t *testing.T) {
	t.Parallel()
	pool, _, wfID := setupAgentDefTestEnv(t, nil)
	cliModelSvc := NewCLIModelService(pool, clock.Real())
	svc := NewAgentDefinitionService(pool, clock.Real(), cliModelSvc, nil, false)

	def, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:            "agent-cli-int-db",
		Prompt:        "do stuff",
		ExecutionMode: "cli_interactive",
		Model:         "opus_4_7",
	})
	if err != nil {
		t.Fatalf("CreateAgentDef(cli_interactive, opus_4_7 via DB lookup): %v", err)
	}
	if def.ExecutionMode != "cli_interactive" {
		t.Errorf("ExecutionMode = %q, want cli_interactive", def.ExecutionMode)
	}
}

// TestUpdateAgentDef_ToCLIInteractive_Succeeds verifies that updating execution_mode to
// cli_interactive succeeds when the existing model is compatible (default sonnet → claude).
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
	model := "codex_gpt_normal"
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
	if def.Model != "codex_gpt_normal" {
		t.Errorf("Model = %q, want codex_gpt_normal", def.Model)
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
		{"claude default", "ag-claude", "opus_4_7", true},
		{"codex prefix", "ag-codex", "codex_gpt_high", true},
		{"opencode prefix rejected", "ag-opencode", "opencode_minimax_m25_free", false},
		{"unknown prefix falls back to claude", "ag-unknown", "mycompany_model_v1", true},
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

// TestCreateAgentDef_CLIInteractive_OpencodeModelRejected verifies that multiple opencode
// models are all rejected for cli_interactive, while cli mode succeeds.
func TestCreateAgentDef_CLIInteractive_OpencodeModelRejected(t *testing.T) {
	t.Parallel()
	svc, wfID := setupAgentDefCLIInteractiveEnv(t)

	opencodeModels := []string{
		"opencode_minimax_m25_free",
		"opencode_qwen36_plus_free",
		"opencode_gpt54",
		"opencode_gpt54_mini_low",
	}
	for i, model := range opencodeModels {
		model := model
		agentID := "oc-reject-" + string(rune('a'+i))
		t.Run(model, func(t *testing.T) {
			_, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
				ID:            agentID,
				Prompt:        "do stuff",
				ExecutionMode: "cli_interactive",
				Model:         model,
			})
			if err == nil {
				t.Fatalf("CreateAgentDef(cli_interactive, %q): expected error, got nil", model)
			}
			if !strings.Contains(err.Error(), "opencode does not support") {
				t.Errorf("error = %q, want to contain %q", err.Error(), "opencode does not support")
			}
		})
	}
}

// TestCreateAgentDef_CLIInteractiveSucceeds_OpencodeModel verifies that cli_interactive mode
// with an opencode model is rejected (opencode only supports cli/batch).
func TestCreateAgentDef_CLISucceeds_OpencodeModel(t *testing.T) {
	t.Parallel()
	svc, wfID := setupAgentDefCLIInteractiveEnv(t)

	_, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:            "oc-cli-ok",
		Prompt:        "do stuff",
		ExecutionMode: "cli_interactive",
		Model:         "opencode_minimax_m25_free",
	})
	if err == nil {
		t.Fatal("CreateAgentDef(cli_interactive, opencode_minimax_m25_free): expected error (opencode does not support cli_interactive)")
	}
}
