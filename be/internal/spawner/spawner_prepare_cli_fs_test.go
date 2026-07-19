package spawner

// Acceptance tests for the cli_interactive Claude native_tools=="none" FS
// bridge path: attachNrfloToolRegistry (mcp_tools.go) merges the jailed FS
// trio (read_file/edit_file/bash) into the bridge registry, bypassing the
// api_native_tools_enabled global. Mirrors spawner_prepare_cli_mcp_test.go's
// harness (cliMCPSpawner + setupContextSaveTestEnv) with a native_tools
// column and a real Config.ProjectRoot so proc.workDir != "".

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
)

// insertCLIAgentDefWithNativeTools inserts a cli_interactive agent_definition
// row with an explicit native_tools value; insertCLIAgentDefWithTools (in
// spawner_prepare_cli_mcp_test.go) omits the column, leaving it at its empty
// default.
func insertCLIAgentDefWithNativeTools(t *testing.T, env *contextSaveTestEnv, agentID, model, tools, nativeTools string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := env.database.Exec(
		`INSERT INTO agent_definitions
			(id, project_id, workflow_id, model, timeout, prompt, execution_mode, tools, native_tools, created_at, updated_at)
		VALUES (?, ?, 'feature', ?, 20, '# prompt', 'cli_interactive', ?, ?, ?, ?)`,
		agentID, env.projectID, model, tools, nativeTools, now, now,
	); err != nil {
		t.Fatalf("insert agent_definition %q: %v", agentID, err)
	}
}

// cliFSSpawner mirrors cliMCPSpawner but also sets Config.ProjectRoot, since
// buildAPIRegistry's FS-merge guard requires proc.workDir != "".
func cliFSSpawner(env *contextSaveTestEnv, projectRoot string) *Spawner {
	return New(Config{
		DataPath:    env.dbPath,
		Pool:        db.WrapAsPool(env.database),
		Clock:       clock.Real(),
		AgentSvc:    &noopAgentSvc{},
		ProjectRoot: projectRoot,
		Workflows: map[string]WorkflowDef{
			"feature": {Phases: []PhaseDef{{NodeID: "impl", Agent: "impl", Layer: 0}}},
		},
	})
}

// TestPrepareSpawn_CLIInteractiveClaude_NativeToolsNone_FSBridge is
// acceptance (1): a claude cli_interactive def resolving native_tools=="none"
// with a default (blank) tools CSV gets the FS trio merged into both the
// bridge tools/list (proc.apiTools) and proc.apiHandlers, and an end-to-end
// edit_file + bash round trip via DispatchTool actually touches the
// filesystem under ProjectRoot.
func TestPrepareSpawn_CLIInteractiveClaude_NativeToolsNone_FSBridge(t *testing.T) {
	t.Parallel()
	ensureTmpNrfloDir(t)
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()
	workDir := t.TempDir()

	insertCLIAgentDefWithNativeTools(t, env, "impl", "sonnet-5", "", "none")

	s := cliFSSpawner(env, workDir)
	proc, prep, err := s.prepareSpawn(context.Background(), SpawnRequest{
		AgentType:          "impl",
		ProjectID:          env.projectID,
		WorkflowName:       "feature",
		WorkflowInstanceID: env.wfiID,
	}, "claude:sonnet-5", "impl", env.wfiID)
	if err != nil {
		t.Fatalf("prepareSpawn() error: %v", err)
	}
	if prep.promptFile != "" {
		t.Cleanup(func() { os.Remove(prep.promptFile) })
	}
	if prep.suffixFile != "" {
		t.Cleanup(func() { os.Remove(prep.suffixFile) })
	}

	if prep.opts.NativeToolsCSV != "none" {
		t.Fatalf("NativeToolsCSV = %q, want %q", prep.opts.NativeToolsCSV, "none")
	}

	var toolListNames []string
	for _, spec := range proc.apiTools {
		toolListNames = append(toolListNames, spec.Name)
	}
	for _, name := range []string{"read_file", "edit_file", "bash"} {
		if _, ok := proc.apiHandlers[name]; !ok {
			t.Errorf("proc.apiHandlers missing %q; want the FS trio merged for native_tools=none", name)
		}
		if !contains(toolListNames, name) {
			t.Errorf("proc.apiTools missing %q; tools/list = %v", name, toolListNames)
		}
	}

	// End-to-end edit_file via DispatchTool: creates a file under ProjectRoot.
	s.registerSessionProc(proc.sessionID, proc)
	editArgs, _ := json.Marshal(map[string]any{
		"path":       "created.txt",
		"old_string": "",
		"new_string": "hello from fs bridge",
	})
	out, _, isErr, terminal, dispatchErr := s.DispatchTool(proc.sessionID, "edit_file", editArgs)
	if dispatchErr != nil {
		t.Fatalf("DispatchTool edit_file error: %v", dispatchErr)
	}
	if isErr {
		t.Fatalf("edit_file isError=true: %s", out)
	}
	if terminal != "" {
		t.Fatalf("edit_file terminal = %q, want empty", terminal)
	}
	data, readErr := os.ReadFile(filepath.Join(workDir, "created.txt"))
	if readErr != nil || string(data) != "hello from fs bridge" {
		t.Fatalf("file content = %q, err=%v, want %q", data, readErr, "hello from fs bridge")
	}

	// End-to-end bash via DispatchTool.
	bashArgs, _ := json.Marshal(map[string]any{"command": "echo hi && cat created.txt"})
	out, _, isErr, terminal, dispatchErr = s.DispatchTool(proc.sessionID, "bash", bashArgs)
	if dispatchErr != nil {
		t.Fatalf("DispatchTool bash error: %v", dispatchErr)
	}
	if isErr {
		t.Fatalf("bash isError=true: %s", out)
	}
	if terminal != "" {
		t.Fatalf("bash terminal = %q, want empty", terminal)
	}
	if !strings.Contains(out, "hi") || !strings.Contains(out, "hello from fs bridge") {
		t.Errorf("bash output = %q, want echo+cat contents", out)
	}
}

// TestPrepareSpawn_CLIInteractiveClaude_NativeToolsUnset_NoFSBridge is
// acceptance (2): a claude cli_interactive def with native_tools left unset
// (the "" default, i.e. unrestricted native CLI tools) must NOT gain the FS
// trio — the registry stays exactly as it was before this feature.
func TestPrepareSpawn_CLIInteractiveClaude_NativeToolsUnset_NoFSBridge(t *testing.T) {
	t.Parallel()
	ensureTmpNrfloDir(t)
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()

	insertCLIAgentDefWithTools(t, env, "impl", "sonnet-5", "")

	s := cliFSSpawner(env, t.TempDir())
	proc, prep, err := s.prepareSpawn(context.Background(), SpawnRequest{
		AgentType:          "impl",
		ProjectID:          env.projectID,
		WorkflowName:       "feature",
		WorkflowInstanceID: env.wfiID,
	}, "claude:sonnet-5", "impl", env.wfiID)
	if err != nil {
		t.Fatalf("prepareSpawn() error: %v", err)
	}
	if prep.promptFile != "" {
		t.Cleanup(func() { os.Remove(prep.promptFile) })
	}
	if prep.suffixFile != "" {
		t.Cleanup(func() { os.Remove(prep.suffixFile) })
	}

	if prep.opts.NativeToolsCSV != "" {
		t.Fatalf("NativeToolsCSV = %q, want empty (native_tools unset)", prep.opts.NativeToolsCSV)
	}
	for _, name := range []string{"read_file", "edit_file", "bash"} {
		if _, ok := proc.apiHandlers[name]; ok {
			t.Errorf("proc.apiHandlers contains %q; native_tools unset must not merge the FS trio", name)
		}
	}
	for _, spec := range proc.apiTools {
		if spec.Name == "read_file" || spec.Name == "edit_file" || spec.Name == "bash" {
			t.Errorf("proc.apiTools contains FS tool %q; native_tools unset must not merge the FS trio", spec.Name)
		}
	}
}

func contains(haystack []string, needle string) bool {
	for _, v := range haystack {
		if v == needle {
			return true
		}
	}
	return false
}
