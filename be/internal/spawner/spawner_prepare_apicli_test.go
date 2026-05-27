package spawner

// Tests for the api-via-cli hybrid spawn path. When Config.APIViaCLI=true and
// the api_models provider is "anthropic", prepareSpawn delegates to
// prepareAPIViaCLISpawn which transforms the spawn into a cli_interactive Claude
// session backed by the nrflo agent mcp bridge.

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/spawner/apirun/provider"
	"be/internal/spawner/apirun/provider/mock"
	"be/internal/spawner/apirun/tools_builtin"
)

// insertAPIAgentDefWithTools inserts an agent_definition row with the given
// tools CSV, allowing tests to exercise specific tool-registry paths.
func insertAPIAgentDefWithTools(t *testing.T, env *contextSaveTestEnv, agentID, model, tools string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := env.database.Exec(
		`INSERT INTO agent_definitions
			(id, project_id, workflow_id, model, timeout, prompt, execution_mode, tools, created_at, updated_at)
		VALUES (?, ?, 'feature', ?, 20, '# prompt', 'api', ?, ?, ?)`,
		agentID, env.projectID, model, tools, now, now,
	); err != nil {
		t.Fatalf("insert agent_definition %q: %v", agentID, err)
	}
}

// ensureTmpNrfloDir creates /tmp/nrflo if it does not exist; prepareAPIViaCLISpawn
// writes temp files there.
func ensureTmpNrfloDir(t *testing.T) {
	t.Helper()
	if err := os.MkdirAll("/tmp/nrflo", 0755); err != nil {
		t.Fatalf("mkdir /tmp/nrflo: %v", err)
	}
}

// TestPrepareSpawn_APIViaCLI_CLIInteractiveMode verifies the happy path of the
// api-via-cli hybrid: the spawn returns cli_interactive mode, wires up the
// ClaudeAdapter, builds the MCP config, and never calls BuildAPIProvider.
func TestPrepareSpawn_APIViaCLI_CLIInteractiveMode(t *testing.T) {
	t.Parallel()
	ensureTmpNrfloDir(t)
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()

	// Include read_document so the path-handler substitution is exercised.
	insertAPIAgentDefWithTools(t, env, "impl", "sonnet", "agent_finished,read_document")

	var buildCalled int
	const contextLen = 150000 // distinct from maxContextForModel default to verify override
	sp := New(Config{
		DataPath:  env.dbPath,
		Pool:      db.WrapAsPool(env.database),
		Clock:     clock.Real(),
		APIMode:   true,
		APIViaCLI: true,
		BuildAPIProvider: func(_ context.Context, _ string, _ string) (provider.Provider, error) {
			buildCalled++
			return mock.New(), nil
		},
		APIModelConfigs: map[string]APIModelConfig{
			"sonnet": {Provider: "anthropic", MappedModel: "claude-sonnet-4-6", ContextLength: contextLen},
		},
		AgentSvc: &noopAgentSvc{},
		Workflows: map[string]WorkflowDef{
			"feature": {Phases: []PhaseDef{{ID: "impl", Agent: "impl", Layer: 0}}},
		},
	})

	proc, prep, err := sp.prepareSpawn(context.Background(), SpawnRequest{
		AgentType:          "impl",
		ProjectID:          env.projectID,
		WorkflowName:       "feature",
		WorkflowInstanceID: env.wfiID,
	}, "claude:sonnet", "impl", env.wfiID)
	if err != nil {
		t.Fatalf("prepareSpawn() error: %v", err)
	}
	if prep.systemPromptOverrideFile != "" {
		t.Cleanup(func() { os.Remove(prep.systemPromptOverrideFile) })
	}
	if prep.promptFile != "" {
		t.Cleanup(func() { os.Remove(prep.promptFile) })
	}

	if prep.executionMode != "cli_interactive" {
		t.Errorf("executionMode = %q, want cli_interactive", prep.executionMode)
	}
	if _, ok := prep.adapter.(*ClaudeAdapter); !ok {
		t.Errorf("prep.adapter type = %T, want *ClaudeAdapter", prep.adapter)
	}
	if prep.opts.NativeToolsCSV != "Read" {
		t.Errorf("NativeToolsCSV = %q, want Read", prep.opts.NativeToolsCSV)
	}
	if prep.opts.AllowedToolsCSV != "mcp__nrflo__* Read" {
		t.Errorf("AllowedToolsCSV = %q, want \"mcp__nrflo__* Read\"", prep.opts.AllowedToolsCSV)
	}
	if prep.suffixFile != "" {
		t.Errorf("suffixFile = %q, want empty for api-via-cli", prep.suffixFile)
	}
	if prep.systemPromptOverrideFile == "" {
		t.Error("systemPromptOverrideFile is empty; want temp file containing system prompt")
	} else {
		b, readErr := os.ReadFile(prep.systemPromptOverrideFile)
		if readErr != nil {
			t.Errorf("ReadFile(systemPromptOverrideFile): %v", readErr)
		} else if string(b) != defaultAPISystemPrompt {
			t.Errorf("system prompt content = %q, want %q", string(b), defaultAPISystemPrompt)
		}
	}
	if prep.promptFile == "" {
		t.Error("promptFile is empty; want non-empty temp file")
	}

	// MCPConfigJSON must encode the nrflo agent mcp bridge command.
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
	if want := resolvedNrfloPath(); srv.Command != want {
		t.Errorf("mcpServers.nrflo.command = %q, want %q (resolved nrflo_server path)", srv.Command, want)
	}
	if len(srv.Args) != 2 || srv.Args[0] != "agent" || srv.Args[1] != "mcp" {
		t.Errorf("mcpServers.nrflo.args = %v, want [agent mcp]", srv.Args)
	}

	// proc fields set by prepareAPIViaCLISpawn
	if proc.maxContext != contextLen {
		t.Errorf("proc.maxContext = %d, want %d (from am.ContextLength)", proc.maxContext, contextLen)
	}
	if proc.nudgeMax != 0 {
		t.Errorf("proc.nudgeMax = %d, want 0 (api-via-cli disables nudge)", proc.nudgeMax)
	}
	if !proc.apiViaCLI {
		t.Error("proc.apiViaCLI = false; want true")
	}
	if len(proc.apiTools) == 0 {
		t.Error("proc.apiTools is empty; want populated registry")
	}
	if proc.apiHandlers == nil {
		t.Error("proc.apiHandlers is nil; want populated registry")
	}

	// read_document must be substituted with the path-returning variant.
	rdh, found := proc.apiHandlers["read_document"]
	if !found {
		t.Error("proc.apiHandlers missing read_document")
	} else if _, isPath := rdh.(tools_builtin.ReadDocumentPathHandler); !isPath {
		t.Errorf("read_document handler type = %T, want tools_builtin.ReadDocumentPathHandler", rdh)
	}

	if buildCalled != 0 {
		t.Errorf("BuildAPIProvider called %d times; want 0 (api-via-cli skips provider build)", buildCalled)
	}
}

// TestPrepareSpawn_APIViaCLI_OpenAIFallsThrough verifies that APIViaCLI=true with
// provider="openai" falls through to the standard in-process api runner because
// only Anthropic models are routed through the CLI.
func TestPrepareSpawn_APIViaCLI_OpenAIFallsThrough(t *testing.T) {
	t.Parallel()
	ensureTmpNrfloDir(t)
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()

	insertAPIAgentDef(t, env, "impl", "gpt-o4")

	var buildCalled int
	sp := New(Config{
		DataPath:  env.dbPath,
		Pool:      db.WrapAsPool(env.database),
		Clock:     clock.Real(),
		APIMode:   true,
		APIViaCLI: true,
		BuildAPIProvider: func(_ context.Context, _ string, _ string) (provider.Provider, error) {
			buildCalled++
			return mock.New(), nil
		},
		APIModelConfigs: map[string]APIModelConfig{
			"gpt-o4": {Provider: "openai", MappedModel: "o4-mini", ContextLength: 128000},
		},
		AgentSvc: &noopAgentSvc{},
		Workflows: map[string]WorkflowDef{
			"feature": {Phases: []PhaseDef{{ID: "impl", Agent: "impl", Layer: 0}}},
		},
	})

	_, prep, err := sp.prepareSpawn(context.Background(), SpawnRequest{
		AgentType:          "impl",
		ProjectID:          env.projectID,
		WorkflowName:       "feature",
		WorkflowInstanceID: env.wfiID,
	}, "claude:gpt-o4", "impl", env.wfiID)
	if err != nil {
		t.Fatalf("prepareSpawn() error: %v", err)
	}
	if prep.executionMode != "api" {
		t.Errorf("executionMode = %q, want api (openai falls through to in-process runner)", prep.executionMode)
	}
	if buildCalled == 0 {
		t.Error("BuildAPIProvider not called; want called for non-anthropic provider")
	}
	if prep.apiProvider == nil {
		t.Error("prep.apiProvider is nil; want set by in-process path")
	}
}

// TestPrepareSpawn_APIViaCLI_Disabled_UsesInProcess verifies that when APIViaCLI=false,
// even an anthropic provider uses the standard in-process api runner.
func TestPrepareSpawn_APIViaCLI_Disabled_UsesInProcess(t *testing.T) {
	t.Parallel()
	ensureTmpNrfloDir(t)
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()

	insertAPIAgentDef(t, env, "impl", "sonnet")

	var buildCalled int
	sp := New(Config{
		DataPath:  env.dbPath,
		Pool:      db.WrapAsPool(env.database),
		Clock:     clock.Real(),
		APIMode:   true,
		APIViaCLI: false,
		BuildAPIProvider: func(_ context.Context, _ string, _ string) (provider.Provider, error) {
			buildCalled++
			return mock.New(), nil
		},
		APIModelConfigs: map[string]APIModelConfig{
			"sonnet": {Provider: "anthropic", MappedModel: "claude-sonnet-4-6", ContextLength: 200000},
		},
		AgentSvc: &noopAgentSvc{},
		Workflows: map[string]WorkflowDef{
			"feature": {Phases: []PhaseDef{{ID: "impl", Agent: "impl", Layer: 0}}},
		},
	})

	_, prep, err := sp.prepareSpawn(context.Background(), SpawnRequest{
		AgentType:          "impl",
		ProjectID:          env.projectID,
		WorkflowName:       "feature",
		WorkflowInstanceID: env.wfiID,
	}, "claude:sonnet", "impl", env.wfiID)
	if err != nil {
		t.Fatalf("prepareSpawn() error: %v", err)
	}
	if prep.executionMode != "api" {
		t.Errorf("executionMode = %q, want api (APIViaCLI disabled)", prep.executionMode)
	}
	if buildCalled == 0 {
		t.Error("BuildAPIProvider not called; want called when APIViaCLI disabled")
	}
}
