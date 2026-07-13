package spawner

// Tests for the cli_interactive MCP wiring: regular Claude cli_interactive agents
// are spawned with the nrflo agent mcp bridge so they call mcp__nrflo__* tools
// instead of the nrflo CLI. Codex (and other non-Claude adapters) are not wired.

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
)

// insertCLIAgentDefWithTools inserts a cli_interactive agent_definition row.
func insertCLIAgentDefWithTools(t *testing.T, env *contextSaveTestEnv, agentID, model, tools string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := env.database.Exec(
		`INSERT INTO agent_definitions
			(id, project_id, workflow_id, model, timeout, prompt, execution_mode, tools, created_at, updated_at)
		VALUES (?, ?, 'feature', ?, 20, '# prompt', 'cli_interactive', ?, ?, ?)`,
		agentID, env.projectID, model, tools, now, now,
	); err != nil {
		t.Fatalf("insert agent_definition %q: %v", agentID, err)
	}
}

func cliMCPSpawner(env *contextSaveTestEnv) *Spawner {
	return New(Config{
		DataPath: env.dbPath,
		Pool:     db.WrapAsPool(env.database),
		Clock:    clock.Real(),
		AgentSvc: &noopAgentSvc{},
		Workflows: map[string]WorkflowDef{
			"feature": {Phases: []PhaseDef{{NodeID: "impl", Agent: "impl", Layer: 0}}},
		},
	})
}

// TestPrepareSpawn_CLIInteractiveClaude_MCPTools verifies a regular Claude
// cli_interactive spawn gets the nrflo MCP bridge, keeps its native tools, and
// always serves the full builtin set even when the agent's tools field is blank.
func TestPrepareSpawn_CLIInteractiveClaude_MCPTools(t *testing.T) {
	t.Parallel()
	ensureTmpNrfloDir(t)
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()

	// Blank tools must still yield the full nrflo tool set (lifecycle/findings),
	// since the CLI path forces "*" — otherwise the agent could never finish.
	insertCLIAgentDefWithTools(t, env, "impl", "sonnet", "")

	proc, prep, err := cliMCPSpawner(env).prepareSpawn(context.Background(), SpawnRequest{
		AgentType:          "impl",
		ProjectID:          env.projectID,
		WorkflowName:       "feature",
		WorkflowInstanceID: env.wfiID,
	}, "claude:sonnet", "impl", env.wfiID)
	if err != nil {
		t.Fatalf("prepareSpawn() error: %v", err)
	}
	if prep.promptFile != "" {
		t.Cleanup(func() { os.Remove(prep.promptFile) })
	}
	if prep.suffixFile != "" {
		t.Cleanup(func() { os.Remove(prep.suffixFile) })
	}

	if prep.executionMode != "cli_interactive" {
		t.Fatalf("executionMode = %q, want cli_interactive", prep.executionMode)
	}
	// Native coding tools must be untouched (no --tools restriction).
	if prep.opts.NativeToolsCSV != "" {
		t.Errorf("NativeToolsCSV = %q, want empty (native tools untouched)", prep.opts.NativeToolsCSV)
	}
	if prep.opts.AllowedToolsCSV != "mcp__nrflo__*" {
		t.Errorf("AllowedToolsCSV = %q, want mcp__nrflo__*", prep.opts.AllowedToolsCSV)
	}
	if proc.apiViaCLI {
		t.Error("proc.apiViaCLI = true; want false for regular cli_interactive Claude")
	}

	// MCP config points at the nrflo agent mcp bridge.
	var mcpCfg struct {
		MCPServers map[string]struct {
			Command string   `json:"command"`
			Args    []string `json:"args"`
		} `json:"mcpServers"`
	}
	if jsonErr := json.Unmarshal([]byte(prep.opts.MCPConfigJSON), &mcpCfg); jsonErr != nil {
		t.Fatalf("unmarshal MCPConfigJSON: %v, raw=%q", jsonErr, prep.opts.MCPConfigJSON)
	}
	srv, ok := mcpCfg.MCPServers["nrflo"]
	if !ok {
		t.Fatal("mcpServers.nrflo missing from MCPConfigJSON")
	}
	if len(srv.Args) != 2 || srv.Args[0] != "agent" || srv.Args[1] != "mcp" {
		t.Errorf("mcpServers.nrflo.args = %v, want [agent mcp]", srv.Args)
	}

	// Lifecycle + findings + chain builtins must be present despite blank tools.
	for _, name := range []string{"agent_finished", "agent_fail", "findings_add", "chain_next_instructions", "chain_next_ticket"} {
		if _, ok := proc.apiHandlers[name]; !ok {
			t.Errorf("proc.apiHandlers missing %q (full builtin set should be served)", name)
		}
	}
	if len(proc.apiTools) == 0 {
		t.Error("proc.apiTools is empty; want populated registry")
	}
}

// TestPrepareSpawn_CLIInteractiveCodex_RegistryNoFlags verifies codex gets the
// nrflo tool registry attached to proc (served later via a config.toml
// [mcp_servers] table written by the app-server backend) but NOT the Claude-only
// --mcp-config/--allowedTools opts.
func TestPrepareSpawn_CLIInteractiveCodex_RegistryNoFlags(t *testing.T) {
	t.Parallel()
	ensureTmpNrfloDir(t)
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()

	insertCLIAgentDefWithTools(t, env, "impl", "gpt-5-codex", "*")

	proc, prep, err := cliMCPSpawner(env).prepareSpawn(context.Background(), SpawnRequest{
		AgentType:          "impl",
		ProjectID:          env.projectID,
		WorkflowName:       "feature",
		WorkflowInstanceID: env.wfiID,
	}, "codex:gpt-5-codex", "impl", env.wfiID)
	if err != nil {
		t.Fatalf("prepareSpawn() error: %v", err)
	}
	if prep.promptFile != "" {
		t.Cleanup(func() { os.Remove(prep.promptFile) })
	}

	// Codex must not carry the Claude-only flags (it consumes config.toml instead).
	if prep.opts.MCPConfigJSON != "" {
		t.Errorf("MCPConfigJSON = %q, want empty for codex (config.toml path)", prep.opts.MCPConfigJSON)
	}
	if prep.opts.AllowedToolsCSV != "" {
		t.Errorf("AllowedToolsCSV = %q, want empty for codex", prep.opts.AllowedToolsCSV)
	}
	// But the registry must be attached so the MCP bridge can serve tools.
	if len(proc.apiTools) == 0 {
		t.Error("proc.apiTools empty for codex; want the nrflo tool registry attached")
	}
	for _, name := range []string{"agent_finished", "findings_add"} {
		if _, ok := proc.apiHandlers[name]; !ok {
			t.Errorf("proc.apiHandlers missing %q for codex", name)
		}
	}
}
