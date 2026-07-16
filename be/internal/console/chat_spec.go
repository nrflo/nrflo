package console

import (
	"fmt"
	"os"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/repo"
	"be/internal/spawner"
)

// chatSpecParams bundles what buildChatEngineSpec needs beyond the DB pool.
type chatSpecParams struct {
	SessionID       string
	ProjectID       string
	Engine          string // "claude" | "codex" | "api"
	ModelID         string // cli_models/api_models registry id, a raw model name, or ""
	ReasoningEffort string // optional override; must be in the row's supported_efforts
	SpawnToken      string
	ServerURL       string // loopback base, e.g. http://127.0.0.1:6587
}

// buildChatEngineSpec resolves the project workdir and (when ModelID names
// one) the model registry row into a spawner.EngineSpec for a console-chat
// session, via modelResolverFor(p.Engine) — cli_models and api_models are
// separate tables whose ids collide, so resolution diverges by engine. A row
// that doesn't resolve is an error surfaced before the engine is started.
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
		// Engine-default model: the effort override passes through for the
		// engine/provider to validate.
		spec.ReasoningEffort = p.ReasoningEffort
		return spec, nil
	}

	if err := modelResolverFor(p.Engine).Resolve(pool, clk, &spec, p.ModelID, p.ReasoningEffort); err != nil {
		return spawner.EngineSpec{}, err
	}

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
// minus nested-Claude markers (a server started from inside a Claude Code
// session would otherwise mark the console CLI as a child session, which
// suppresses its transcript JSONL — the claude engine's only text source),
// plus session identity. Claude's hooks shell out to `nrflo_server agent
// record-event --console` (spawner/hooks_settings_console.go), which needs
// NRF_SESSION_ID and the socket vars to reach back into this server.
func chatEnv(sessionID, projectID string) []string {
	return append(spawner.HostEnvWithoutClaudeMarkers(),
		"NRF_SESSION_ID="+sessionID,
		"NRFLO_PROJECT="+projectID,
	)
}

// chatMCPEnv builds the env the injected `agent mcp-external` bridge needs to
// adopt this pre-minted chat session instead of exchanging its own — the same
// shape the external MCP bridge uses for an adopted console session.
func chatMCPEnv(serverURL, projectID, sessionID, spawnToken string) map[string]string {
	return map[string]string{
		"NRFLO_SERVER_URL":         serverURL,
		"NRFLO_PROJECT":            projectID,
		"NRFLO_CONSOLE_TOKEN":      spawnToken,
		"NRFLO_CONSOLE_SESSION_ID": sessionID,
	}
}
