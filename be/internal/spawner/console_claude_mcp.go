package spawner

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// consoleClaudeMCPConfig mirrors the shape `claude --mcp-config <file>` expects.
type consoleClaudeMCPConfig struct {
	MCPServers map[string]consoleClaudeMCPServer `json:"mcpServers"`
}

type consoleClaudeMCPServer struct {
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
}

// WriteConsoleClaudeMCPConfig writes the `--mcp-config` JSON file a claude
// console launch needs to reach nrflo's tool bridge: a single "nrflo" server
// entry invoking serverPath with args, with env embedded (bearer token never
// in argv — argv is visible to any local user via `ps`). Sibling of
// WriteConsoleCodexProfile; used by both the console claude driver
// (console/driver_claude.go) and the claude console engine.
func WriteConsoleClaudeMCPConfig(dir, serverPath string, args []string, env map[string]string) (string, error) {
	cfg := consoleClaudeMCPConfig{MCPServers: map[string]consoleClaudeMCPServer{
		"nrflo": {
			Command: serverPath,
			Args:    args,
			Env:     env,
		},
	}}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, "mcp-config.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", err
	}
	return path, nil
}
