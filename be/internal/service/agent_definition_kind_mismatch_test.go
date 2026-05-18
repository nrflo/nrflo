package service

import (
	"strings"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/repo"
	"be/internal/types"
)

// setupAgentDefWithToolScript creates an env with a project, workflow, and a
// python_script with kind='tool'. Returns service, workflowID, and toolScriptID.
func setupAgentDefWithToolScript(t *testing.T) (*AgentDefinitionService, string, string) {
	t.Helper()
	pool, _, wfID := setupAgentDefTestEnv(t, nil)

	toolScriptID := "ps-tool-kind"
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := pool.Exec(
		`INSERT INTO python_scripts (id, project_id, name, description, code, kind, tool_description, created_at, updated_at)
		 VALUES (?, ?, ?, '', '', 'tool', 'Does useful work', ?, ?)`,
		toolScriptID, "proj1", "my-tool", now, now,
	); err != nil {
		t.Fatalf("insert tool python_script: %v", err)
	}

	scriptRepo := repo.NewPythonScriptRepo(pool, clock.Real())
	cliModelSvc := NewCLIModelService(pool, clock.Real())
	svc := NewAgentDefinitionService(pool, clock.Real(), cliModelSvc, scriptRepo)
	return svc, wfID, toolScriptID
}

// TestCreateAgentDef_ScriptMode_ToolKindScriptRejected verifies that providing
// a python_script with kind='tool' in script execution_mode returns python_script_kind_mismatch.
func TestCreateAgentDef_ScriptMode_ToolKindScriptRejected(t *testing.T) {
	t.Parallel()
	svc, wfID, toolScriptID := setupAgentDefWithToolScript(t)

	_, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:             "agent-tool-kind",
		ExecutionMode:  "script",
		PythonScriptID: &toolScriptID,
	})
	if err == nil {
		t.Fatal("CreateAgentDef with tool-kind script: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "python_script_kind_mismatch") {
		t.Errorf("error = %q, want to contain 'python_script_kind_mismatch'", err.Error())
	}
}

// TestUpdateAgentDef_ScriptMode_ToolKindScriptRejected verifies that switching to
// script mode with a tool-kind script is rejected on UpdateAgentDef.
func TestUpdateAgentDef_ScriptMode_ToolKindScriptRejected(t *testing.T) {
	t.Parallel()
	svc, wfID, toolScriptID := setupAgentDefWithToolScript(t)

	// First create a CLI agent.
	_, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:            "agent-upd-tool-kind",
		ExecutionMode: "cli_interactive",
		Prompt:        "do stuff",
	})
	if err != nil {
		t.Fatalf("create CLI agent: %v", err)
	}

	scriptMode := "script"
	emptyPrompt := ""
	err = svc.UpdateAgentDef("proj1", wfID, "agent-upd-tool-kind", &types.AgentDefUpdateRequest{
		ExecutionMode:  &scriptMode,
		Prompt:         &emptyPrompt,
		PythonScriptID: &toolScriptID,
	})
	if err == nil {
		t.Fatal("UpdateAgentDef switch to script mode with tool-kind script: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "python_script_kind_mismatch") {
		t.Errorf("error = %q, want to contain 'python_script_kind_mismatch'", err.Error())
	}
}

// TestUpdateAgentDef_PythonScriptID_ToolKindScriptRejected verifies that updating
// python_script_id on an existing script-mode agent to a tool-kind script is rejected.
func TestUpdateAgentDef_PythonScriptID_ToolKindScriptRejected(t *testing.T) {
	t.Parallel()
	pool, _, wfID := setupAgentDefTestEnv(t, nil)

	now := time.Now().UTC().Format(time.RFC3339Nano)
	agentScriptID := "ps-agent-kind"
	toolScriptID := "ps-tool-kind2"

	if _, err := pool.Exec(
		`INSERT INTO python_scripts (id, project_id, name, description, code, kind, created_at, updated_at)
		 VALUES (?, ?, ?, '', '', 'agent', ?, ?)`,
		agentScriptID, "proj1", "my-agent-script", now, now,
	); err != nil {
		t.Fatalf("insert agent python_script: %v", err)
	}
	if _, err := pool.Exec(
		`INSERT INTO python_scripts (id, project_id, name, description, code, kind, tool_description, created_at, updated_at)
		 VALUES (?, ?, ?, '', '', 'tool', 'x', ?, ?)`,
		toolScriptID, "proj1", "my-tool-script", now, now,
	); err != nil {
		t.Fatalf("insert tool python_script: %v", err)
	}

	scriptRepo := repo.NewPythonScriptRepo(pool, clock.Real())
	cliModelSvc := NewCLIModelService(pool, clock.Real())
	svc := NewAgentDefinitionService(pool, clock.Real(), cliModelSvc, scriptRepo)

	// Create a script-mode agent pointing at the agent-kind script.
	_, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:             "agent-swap-kind",
		ExecutionMode:  "script",
		PythonScriptID: &agentScriptID,
	})
	if err != nil {
		t.Fatalf("create script-mode agent: %v", err)
	}

	// Now try to update to point at the tool-kind script.
	err = svc.UpdateAgentDef("proj1", wfID, "agent-swap-kind", &types.AgentDefUpdateRequest{
		PythonScriptID: &toolScriptID,
	})
	if err == nil {
		t.Fatal("UpdateAgentDef set python_script_id to tool-kind: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "python_script_kind_mismatch") {
		t.Errorf("error = %q, want to contain 'python_script_kind_mismatch'", err.Error())
	}
}
