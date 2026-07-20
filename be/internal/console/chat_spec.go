package console

import (
	"context"
	"fmt"
	"os"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/spawner"
)

// chatSpecParams bundles what buildChatEngineSpec needs beyond the DB pool.
type chatSpecParams struct {
	SessionID        string
	ProjectID        string
	Engine           string // "claude" | "codex" | "api"
	ModelID          string // models registry id, a raw CLI model name, or ""
	ReasoningEffort  string // optional override; must be in the selected mode's effort list
	SpawnToken       string
	ServerURL        string // loopback base, e.g. http://127.0.0.1:6587
	SystemTemplateID string // optional agent-def/profile injectable id, rendered into spec.SystemPrompt

	// Profile-derived fields (console.Profile, resolved by the caller —
	// chat_service.go's create()). Zero values are "no profile", byte-
	// identical to pre-profile behavior. NativeToolsCSV/Sandbox are the raw
	// spawner.EngineSpec values mapped from Profile.NativeToolPolicy by
	// nativeToolFieldsForPolicy; NativeToolPolicy passes the policy through
	// unchanged for the api engine's own fs-tool gate.
	NativeToolsCSV      string
	Sandbox             string
	NativeToolPolicy    string
	ContextBudgetTokens int
	// DefaultModelID/DefaultEffort apply only when the caller left
	// ModelID/ReasoningEffort empty (a profile default, not an override).
	DefaultModelID string
	DefaultEffort  string
}

// nativeToolFieldsForPolicy maps a Profile.NativeToolPolicy onto the raw
// spawner.EngineSpec fields each CLI engine reads: "none" locks the claude
// engine to MCP-only tools (model.NativeToolsNone, console_engine_claude.go)
// and the codex engine to a read-only sandbox; "full"/"" leave both at their
// engine default (unrestricted).
func nativeToolFieldsForPolicy(policy string) (nativeToolsCSV, sandbox string) {
	if policy == NativeToolPolicyNone {
		return model.NativeToolsNone, model.SandboxReadOnly
	}
	return "", ""
}

// buildChatEngineSpec resolves the project workdir and (when ModelID names
// one, or falls back to DefaultModelID) the model registry row into a
// spawner.EngineSpec for a console-chat session, via
// modelResolverFor(p.Engine). API rows must resolve; unknown CLI ids remain
// valid raw model names.
func buildChatEngineSpec(pool *db.Pool, clk clock.Clock, p chatSpecParams) (spawner.EngineSpec, error) {
	project, err := repo.NewProjectRepo(pool, clk).Get(p.ProjectID)
	if err != nil {
		return spawner.EngineSpec{}, fmt.Errorf("get project: %w", err)
	}
	workDir := ""
	if project.RootPath.Valid {
		workDir = project.RootPath.String
	}

	nativeToolsCSV, sandbox := nativeToolFieldsForPolicy(p.NativeToolPolicy)
	spec := spawner.EngineSpec{
		SessionID:           p.SessionID,
		ProjectID:           p.ProjectID,
		WorkDir:             workDir,
		Model:               p.ModelID,
		MCPServerPath:       resolveNrfloPath(),
		Env:                 chatEnv(p.SessionID, p.ProjectID),
		MCPEnv:              chatMCPEnv(p.ServerURL, p.ProjectID, p.SessionID, p.SpawnToken),
		NativeToolsCSV:      nativeToolsCSV,
		Sandbox:             sandbox,
		NativeToolPolicy:    p.NativeToolPolicy,
		ContextBudgetTokens: p.ContextBudgetTokens,
	}

	modelID := p.ModelID
	if modelID == "" {
		modelID = p.DefaultModelID
	}
	effort := p.ReasoningEffort
	if effort == "" {
		effort = p.DefaultEffort
	}

	if modelID == "" {
		// Engine-default model: the effort override passes through for the
		// engine/provider to validate.
		spec.ReasoningEffort = effort
	} else if err := modelResolverFor(p.Engine).Resolve(pool, clk, &spec, modelID, effort); err != nil {
		return spawner.EngineSpec{}, err
	}

	if p.SystemTemplateID != "" {
		vars := map[string]string{"PROJECT_ID": p.ProjectID, "MODEL": spec.Model, "NODE_ID": p.SessionID}
		spec.SystemPrompt = spawner.RenderInjectable(context.Background(), pool, p.SystemTemplateID, vars)
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
