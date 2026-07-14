package console

import (
	"fmt"
	"os"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/repo"
	"be/internal/service"
	"be/internal/spawner"
)

// chatSpecParams bundles what buildChatEngineSpec needs beyond the DB pool.
type chatSpecParams struct {
	SessionID  string
	ProjectID  string
	Engine     string // "claude" | "codex"
	ModelID    string // cli_models registry id, a raw model name, or ""
	SpawnToken string
	ServerURL  string // loopback base, e.g. http://127.0.0.1:6587
}

// buildChatEngineSpec resolves the project workdir and (when ModelID names
// one) the cli_models row into a spawner.EngineSpec for a console-chat
// session. A ModelID absent from the registry passes through raw — still a
// legal CLI model name — mirroring cli/console_client.go's resolveCLIModel. A
// row that exists but belongs to another engine, or is disabled, is an error
// surfaced before the engine is started.
func buildChatEngineSpec(pool *db.Pool, clk clock.Clock, p chatSpecParams) (spawner.EngineSpec, error) {
	project, err := repo.NewProjectRepo(pool, clk).Get(p.ProjectID)
	if err != nil {
		return spawner.EngineSpec{}, fmt.Errorf("get project: %w", err)
	}
	workDir := ""
	if project.RootPath.Valid {
		workDir = project.RootPath.String
	}

	spec := spawner.EngineSpec{
		SessionID:     p.SessionID,
		ProjectID:     p.ProjectID,
		WorkDir:       workDir,
		Model:         p.ModelID,
		MCPServerPath: resolveNrfloPath(),
		Env:           chatEnv(p.SessionID, p.ProjectID),
		MCPEnv:        chatMCPEnv(p.ServerURL, p.ProjectID, p.SessionID, p.SpawnToken),
	}

	if p.ModelID == "" {
		return spec, nil
	}

	row, getErr := service.NewCLIModelService(pool, clk).Get(p.ModelID)
	if getErr != nil {
		// Unknown id: keep spec.Model as the raw value, no effort/fallback.
		return spec, nil
	}
	if row.CLIType != p.Engine {
		return spawner.EngineSpec{}, fmt.Errorf("model %q is registered for cli %s, not %s", p.ModelID, row.CLIType, p.Engine)
	}
	if !row.Enabled {
		return spawner.EngineSpec{}, fmt.Errorf("model %q is disabled in the cli_models registry", p.ModelID)
	}
	spec.Model = row.MappedModel
	spec.ReasoningEffort = row.ReasoningEffort
	spec.FallbackModels = row.FallbackModels
	spec.MaxContext = row.ContextLength

	return spec, nil
}

// resolveNrfloPath returns the absolute path to the running nrflo_server
// binary — mirrors spawner.resolvedNrfloPath (unexported to that package) —
// used both as EngineSpec.MCPServerPath (codexEngine) and EngineDeps.NrfloPath
// (claudeEngine writes its --mcp-config from the deps value, not the spec one).
func resolveNrfloPath() string {
	if exe, err := os.Executable(); err == nil {
		return exe
	}
	return "nrflo_server"
}

// chatEnv builds the process env for the console-chat CLI: the running
// server's own env (so its hooks resolve the same NRFLO_SOCKET/NRFLO_HOME)
// plus session identity. Claude's hooks shell out to `nrflo_server agent
// record-event --console` (spawner/hooks_settings_console.go), which needs
// NRF_SESSION_ID and the socket vars to reach back into this server.
func chatEnv(sessionID, projectID string) []string {
	env := append([]string{}, os.Environ()...)
	return append(env,
		"NRF_SESSION_ID="+sessionID,
		"NRFLO_PROJECT="+projectID,
	)
}

// chatMCPEnv builds the env the injected `agent mcp-external` bridge needs to
// adopt this pre-minted chat session instead of exchanging its own — the same
// shape console/driver.go's bridgeEnv uses for a human console session.
func chatMCPEnv(serverURL, projectID, sessionID, spawnToken string) map[string]string {
	return map[string]string{
		"NRFLO_SERVER_URL":         serverURL,
		"NRFLO_PROJECT":            projectID,
		"NRFLO_CONSOLE_TOKEN":      spawnToken,
		"NRFLO_CONSOLE_SESSION_ID": sessionID,
	}
}
