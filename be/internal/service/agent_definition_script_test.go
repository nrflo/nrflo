package service

import (
	"strings"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/repo"
	"be/internal/types"
)

// setupAgentDefScriptEnv creates an isolated DB with a project, workflow, and a
// stored Python script. Returns pool, service (apiMode=false), workflowID, and scriptID.
// The service has a real PythonScriptRepo wired for cross-project validation.
func setupAgentDefScriptEnv(t *testing.T) (*AgentDefinitionService, string, string) {
	t.Helper()
	pool, _, wfID := setupAgentDefTestEnv(t, nil)

	scriptID := "ps-scripttest"
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := pool.Exec(
		`INSERT INTO python_scripts (id, project_id, name, description, code, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		scriptID, "proj1", "test-script", "", "print('hello')", now, now,
	); err != nil {
		t.Fatalf("insert python_scripts: %v", err)
	}

	scriptRepo := repo.NewPythonScriptRepo(pool, clock.Real())
	cliModelSvc := NewCLIModelService(pool, clock.Real())
	svc := NewAgentDefinitionService(pool, clock.Real(), cliModelSvc, scriptRepo, false)
	return svc, wfID, scriptID
}

// --- CreateAgentDef script mode validation errors (one per error key) ---

// TestCreateAgentDef_ScriptMode_ValidationErrors uses table-driven subtests to
// verify each coupling rule returns the expected error key.
func TestCreateAgentDef_ScriptMode_ValidationErrors(t *testing.T) {
	t.Parallel()
	svc, wfID, scriptID := setupAgentDefScriptEnv(t)

	apiIter := 1
	tests := []struct {
		name    string
		req     types.AgentDefCreateRequest
		wantErr string
	}{
		{
			name: "missing_python_script_id",
			req: types.AgentDefCreateRequest{
				ID:            "agent-err-1",
				ExecutionMode: "script",
				// PythonScriptID intentionally nil
			},
			wantErr: "python_script_id_required",
		},
		{
			name: "prompt_not_empty",
			req: types.AgentDefCreateRequest{
				ID:             "agent-err-2",
				ExecutionMode:  "script",
				PythonScriptID: &scriptID,
				Prompt:         "do stuff",
			},
			wantErr: "script_mode_no_prompt",
		},
		{
			name: "tools_not_empty",
			req: types.AgentDefCreateRequest{
				ID:             "agent-err-3",
				ExecutionMode:  "script",
				PythonScriptID: &scriptID,
				Tools:          "some-tool",
			},
			wantErr: "script_mode_no_tools",
		},
		{
			name: "api_max_iterations_not_nil",
			req: types.AgentDefCreateRequest{
				ID:               "agent-err-4",
				ExecutionMode:    "script",
				PythonScriptID:   &scriptID,
				APIMaxIterations: &apiIter,
			},
			wantErr: "script_mode_no_api_max_iterations",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.CreateAgentDef("proj1", wfID, &tc.req)
			if err == nil {
				t.Fatalf("CreateAgentDef(%s): expected error %q, got nil", tc.name, tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("CreateAgentDef(%s) error = %q, want to contain %q", tc.name, err.Error(), tc.wantErr)
			}
		})
	}
}

// TestCreateAgentDef_ScriptMode_CrossProjectScript verifies that referencing a
// script that does not belong to the current project returns python_script_not_found.
func TestCreateAgentDef_ScriptMode_CrossProjectScript(t *testing.T) {
	t.Parallel()
	svc, wfID, _ := setupAgentDefScriptEnv(t)

	nonExistent := "ps-does-not-exist"
	_, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:             "agent-cross",
		ExecutionMode:  "script",
		PythonScriptID: &nonExistent,
	})
	if err == nil {
		t.Fatal("expected error for non-existent script, got nil")
	}
	if !strings.Contains(err.Error(), "python_script_not_found") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "python_script_not_found")
	}
	if !strings.Contains(err.Error(), nonExistent) {
		t.Errorf("error = %q, want to contain script ID %q", err.Error(), nonExistent)
	}
}

// TestCreateAgentDef_ScriptMode_ModelForcedToScript verifies that the persisted
// model is always "script" regardless of what the caller supplies.
func TestCreateAgentDef_ScriptMode_ModelForcedToScript(t *testing.T) {
	t.Parallel()
	svc, wfID, scriptID := setupAgentDefScriptEnv(t)

	for _, model := range []string{"sonnet", "opus_4_7", ""} {
		req := &types.AgentDefCreateRequest{
			ID:             "agent-model-" + model,
			ExecutionMode:  "script",
			PythonScriptID: &scriptID,
			Model:          model,
		}
		def, err := svc.CreateAgentDef("proj1", wfID, req)
		if err != nil {
			t.Fatalf("CreateAgentDef(model=%q): %v", model, err)
		}
		if def.Model != "script" {
			t.Errorf("model=%q → persisted Model = %q, want %q", model, def.Model, "script")
		}
	}
}

// TestCreateAgentDef_ScriptMode_Success_APIModeFalse verifies that script mode works
// even when the service was constructed with apiMode=false (script is not API-gated).
func TestCreateAgentDef_ScriptMode_Success_APIModeFalse(t *testing.T) {
	t.Parallel()
	svc, wfID, scriptID := setupAgentDefScriptEnv(t)

	def, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:             "agent-script-off",
		ExecutionMode:  "script",
		PythonScriptID: &scriptID,
	})
	if err != nil {
		t.Fatalf("CreateAgentDef(script, apiMode=false): %v", err)
	}
	if def.ExecutionMode != "script" {
		t.Errorf("ExecutionMode = %q, want %q", def.ExecutionMode, "script")
	}
	if def.PythonScriptID == nil || *def.PythonScriptID != scriptID {
		t.Errorf("PythonScriptID = %v, want %q", def.PythonScriptID, scriptID)
	}
}

// TestCreateAgentDef_ScriptMode_Success_APIModeTrue verifies that script mode
// also works when the service was constructed with apiMode=true.
func TestCreateAgentDef_ScriptMode_Success_APIModeTrue(t *testing.T) {
	t.Parallel()
	pool, _, wfID := setupAgentDefTestEnv(t, nil)
	scriptID := "ps-apimodeon"
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := pool.Exec(
		`INSERT INTO python_scripts (id, project_id, name, description, code, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		scriptID, "proj1", "script-on", "", "print(1)", now, now,
	); err != nil {
		t.Fatalf("insert python_scripts: %v", err)
	}
	scriptRepo := repo.NewPythonScriptRepo(pool, clock.Real())
	cliModelSvc := NewCLIModelService(pool, clock.Real())
	svc := NewAgentDefinitionService(pool, clock.Real(), cliModelSvc, scriptRepo, true)

	def, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:             "agent-script-on",
		ExecutionMode:  "script",
		PythonScriptID: &scriptID,
	})
	if err != nil {
		t.Fatalf("CreateAgentDef(script, apiMode=true): %v", err)
	}
	if def.ExecutionMode != "script" {
		t.Errorf("ExecutionMode = %q, want %q", def.ExecutionMode, "script")
	}
}

// TestCreateAgentDef_CLIMode_PythonScriptIDRejected verifies that supplying
// python_script_id for a non-script agent returns python_script_id_requires_script_mode.
func TestCreateAgentDef_CLIMode_PythonScriptIDRejected(t *testing.T) {
	t.Parallel()
	svc, wfID, scriptID := setupAgentDefScriptEnv(t)

	_, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:             "agent-cli-with-script",
		ExecutionMode:  "cli_interactive",
		Prompt:         "do stuff",
		PythonScriptID: &scriptID,
	})
	if err == nil {
		t.Fatal("expected error for python_script_id on cli mode, got nil")
	}
	if !strings.Contains(err.Error(), "python_script_id_requires_script_mode") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "python_script_id_requires_script_mode")
	}
}

// TestCreateAgentDef_ScriptMode_StallStartDefaultZero verifies that script agents
// default stall_start_timeout to 0 (disabled) when not explicitly set.
func TestCreateAgentDef_ScriptMode_StallStartDefaultZero(t *testing.T) {
	t.Parallel()
	svc, wfID, scriptID := setupAgentDefScriptEnv(t)

	def, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:             "agent-stall-default",
		ExecutionMode:  "script",
		PythonScriptID: &scriptID,
	})
	if err != nil {
		t.Fatalf("CreateAgentDef: %v", err)
	}
	if def.StallStartTimeoutSec == nil {
		t.Fatalf("StallStartTimeoutSec = nil, want *0")
	}
	if *def.StallStartTimeoutSec != 0 {
		t.Errorf("StallStartTimeoutSec = %d, want 0 (disabled by default for script agents)", *def.StallStartTimeoutSec)
	}
}

// --- UpdateAgentDef script mode tests ---

// TestUpdateAgentDef_SwitchToScriptMode_Success verifies that an existing CLI agent
// can be updated to script mode with a valid python_script_id.
func TestUpdateAgentDef_SwitchToScriptMode_Success(t *testing.T) {
	t.Parallel()
	svc, wfID, scriptID := setupAgentDefScriptEnv(t)

	// Create a cli_interactive agent first.
	_, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:            "upd-to-script",
		ExecutionMode: "cli_interactive",
		Prompt:        "do stuff",
	})
	if err != nil {
		t.Fatalf("create CLI agent: %v", err)
	}

	scriptMode := "script"
	emptyPrompt := ""
	if err := svc.UpdateAgentDef("proj1", wfID, "upd-to-script", &types.AgentDefUpdateRequest{
		ExecutionMode:  &scriptMode,
		Prompt:         &emptyPrompt,
		PythonScriptID: &scriptID,
	}); err != nil {
		t.Fatalf("UpdateAgentDef to script mode: %v", err)
	}

	def, err := svc.GetAgentDef("proj1", wfID, "upd-to-script")
	if err != nil {
		t.Fatalf("GetAgentDef: %v", err)
	}
	if def.ExecutionMode != "script" {
		t.Errorf("ExecutionMode = %q, want %q", def.ExecutionMode, "script")
	}
	if def.Model != "script" {
		t.Errorf("Model = %q, want %q (model sentinel forced on switch to script)", def.Model, "script")
	}
	if def.PythonScriptID == nil || *def.PythonScriptID != scriptID {
		t.Errorf("PythonScriptID = %v, want %q", def.PythonScriptID, scriptID)
	}
}

// TestUpdateAgentDef_PythonScriptID_OnNonScriptMode_Fails verifies that updating
// python_script_id on a CLI agent (without also switching mode) is rejected.
func TestUpdateAgentDef_PythonScriptID_OnNonScriptMode_Fails(t *testing.T) {
	t.Parallel()
	svc, wfID, scriptID := setupAgentDefScriptEnv(t)

	_, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:            "upd-cli-scriptid",
		ExecutionMode: "cli_interactive",
		Prompt:        "do stuff",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	err = svc.UpdateAgentDef("proj1", wfID, "upd-cli-scriptid", &types.AgentDefUpdateRequest{
		PythonScriptID: &scriptID,
		// ExecutionMode intentionally not set — stays "cli_interactive"
	})
	if err == nil {
		t.Fatal("expected error when setting python_script_id on cli mode, got nil")
	}
	if !strings.Contains(err.Error(), "python_script_id_requires_script_mode") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "python_script_id_requires_script_mode")
	}
}
