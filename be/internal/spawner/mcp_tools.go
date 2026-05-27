package spawner

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"be/internal/model"
	"be/internal/spawner/apirun"
	"be/internal/spawner/apirun/provider"
	"be/internal/spawner/apirun/tools_builtin"
)

// buildNrfloMCPConfig returns the --mcp-config JSON that registers the nrflo
// agent mcp stdio bridge with the spawned Claude process. The bridge proxies
// tools/list + tools/call back over the Unix socket to this server's tool
// registry, so the agent calls mcp__nrflo__* tools instead of the nrflo CLI.
func buildNrfloMCPConfig() (string, error) {
	cfg, err := json.Marshal(map[string]interface{}{
		"mcpServers": map[string]interface{}{
			"nrflo": map[string]interface{}{
				"command": resolvedNrfloPath(),
				"args":    []string{"agent", "mcp"},
			},
		},
	})
	if err != nil {
		return "", err
	}
	return string(cfg), nil
}

// substituteReadDocumentPath swaps the read_document handler/spec for the
// path-returning variant. Claude has a native Read tool and artifacts are
// pre-materialized to NRF_ARTIFACTS_DIR, so returning a path is cheaper than
// inlining bytes. No-op when read_document is not in the registry.
func substituteReadDocumentPath(specs []provider.ToolSpec, handlers apirun.Registry) {
	if _, ok := handlers["read_document"]; !ok {
		return
	}
	pathHandler := tools_builtin.ReadDocumentPathHandler{}
	handlers["read_document"] = pathHandler
	for i, spec := range specs {
		if spec.Name == "read_document" {
			specs[i] = pathHandler.Spec()
			break
		}
	}
}

// attachNrfloToolRegistry builds the nrflo tool registry for a cli_interactive
// spawn and attaches it to proc so the MCP bridge can serve tools/list +
// tools/call. The agent definition's tools field is honored (empty → "*", the
// full set, for backward compatibility); the agent_* lifecycle baseline is
// force-merged so a restrictive tools CSV can never strip an agent's ability to
// signal findings/lifecycle. read_document is swapped to the path-returning
// variant (the CLI agent reads files natively). Used for both Claude (which
// gets --mcp-config) and codex (config.toml).
func (s *Spawner) attachNrfloToolRegistry(
	ctx context.Context,
	req SpawnRequest,
	wfiID string,
	agentDef *model.AgentDefinition,
	proc *processInfo,
) error {
	toolsCSV := "*"
	if agentDef != nil && strings.TrimSpace(agentDef.Tools) != "" {
		toolsCSV = agentDef.Tools
	}
	specs, handlers, toolEnv, regErr := s.buildAPIRegistry(ctx, req, wfiID, agentDef, proc, toolsCSV, true)
	if regErr != nil {
		return regErr
	}
	substituteReadDocumentPath(specs, handlers)
	proc.apiTools = specs
	proc.apiHandlers = handlers
	proc.apiToolEnv = toolEnv
	return nil
}

// configureClaudeMCPTools attaches the registry (attachNrfloToolRegistry) and
// returns the --mcp-config + --allowedTools values for a Claude spawn. Native
// coding tools are left untouched (NativeToolsCSV stays empty).
func (s *Spawner) configureClaudeMCPTools(
	ctx context.Context,
	req SpawnRequest,
	wfiID string,
	agentDef *model.AgentDefinition,
	proc *processInfo,
) (mcpConfigJSON, allowedToolsCSV string, err error) {
	if regErr := s.attachNrfloToolRegistry(ctx, req, wfiID, agentDef, proc); regErr != nil {
		return "", "", regErr
	}
	cfg, cfgErr := buildNrfloMCPConfig()
	if cfgErr != nil {
		return "", "", fmt.Errorf("build mcp config: %w", cfgErr)
	}
	return cfg, "mcp__nrflo__*", nil
}

// nrfloBridgeEnv returns the env the `nrflo_server agent mcp` bridge subprocess
// needs to reach the running server's socket and identify the session. Claude
// inherits these from the spawn env, but codex does NOT forward parent env to
// MCP server subprocesses, so they are embedded in codex's config.toml.
func nrfloBridgeEnv(sessionID, instanceID, projectID string) map[string]string {
	env := map[string]string{
		"NRF_SESSION_ID":           sessionID,
		"NRF_WORKFLOW_INSTANCE_ID": instanceID,
		"NRFLO_PROJECT":            projectID,
	}
	// Socket resolution: NRFLO_SOCKET if set, else $NRFLO_HOME/agent.sock, else
	// $HOME/.nrflo/agent.sock. Propagate whichever the server has so the bridge
	// resolves the same address.
	for _, k := range []string{"NRFLO_SOCKET", "NRFLO_HOME", "HOME"} {
		if v := os.Getenv(k); v != "" {
			env[k] = v
		}
	}
	return env
}
