package spawner

import (
	"context"
	"fmt"
	"os"

	"be/internal/logger"
	"be/internal/model"
	"be/internal/repo"
)

// prepareAPIViaCLISpawn transforms an api-mode spawn into a cli_interactive spawn
// that drives Claude via PTY, using the nrflo agent mcp bridge to serve the
// api-mode tool registry to the Claude process. Called from the api branch of
// prepareSpawn when Config.APIViaCLI is true and the provider is "anthropic".
// Does NOT call BuildAPIProvider.
func (s *Spawner) prepareAPIViaCLISpawn(
	ctx context.Context,
	req SpawnRequest,
	wfiID, agentID, sessionID, spawnToken string,
	effectiveThreshold int,
	extID, extCtx string,
	prompt string,
	am APIModelConfig,
	agentDef *model.AgentDefinition,
	proc *processInfo,
	prep *prepResult,
) (*processInfo, *prepResult, error) {
	adapter, err := GetCLIAdapter("claude")
	if err != nil {
		return nil, nil, fmt.Errorf("api-via-cli: %w", err)
	}

	// Set maxContext from the api_models row.
	proc.maxContext = am.ContextLength

	// Build tool registry (same as normal api branch).
	specs, handlers, toolEnv, regErr := s.buildAPIRegistry(ctx, req, wfiID, agentDef, proc, "")
	if regErr != nil {
		return nil, nil, regErr
	}

	// Substitute read_document with the path-returning variant: Claude has native
	// Read so it can access materialized artifacts by path directly.
	substituteReadDocumentPath(specs, handlers)

	proc.apiViaCLI = true
	proc.nudgeMax = 0
	proc.adapter = adapter
	// modelID must start "claude:" so BuildInteractiveSettingsJSON emits hooks+statusLine
	// and startBackend routes to the PTY backend (not codex app-server). The incoming
	// modelID is "<cliForModel>:<model>" (e.g. "claude:sonnet"), not "api:" — parse it.
	_, rawModel := parseModelID(proc.modelID)
	claudeModel := am.MappedModel
	if claudeModel == "" {
		claudeModel = adapter.MapModel(rawModel)
	}
	proc.modelID = "claude:" + rawModel

	proc.apiTools = specs
	proc.apiHandlers = handlers
	proc.apiToolEnv = toolEnv

	// Pre-materialize artifacts so Claude can access them by path.
	cliStageDir, _ := EnsureStageDir(req.ProjectID, wfiID)
	if s.config.ArtifactSvc != nil {
		if pool := s.pool(); pool != nil {
			if storage, storageErr := s.config.ArtifactSvc.GetStorage(ctx, req.ProjectID); storageErr == nil {
				if _, matErr := MaterializeAll(ctx, wfiID, req.ProjectID, repo.NewArtifactRepo(pool, s.config.Clock), storage); matErr != nil {
					logger.Warn(ctx, "artifact pre-materialize failed during api-via-cli spawn", "error", matErr)
				}
			} else {
				logger.Warn(ctx, "artifact storage unavailable during api-via-cli spawn", "error", storageErr)
			}
		}
	}

	// Write defaultAPISystemPrompt to a temp file for --system-prompt-file.
	spf, spfErr := os.CreateTemp("/tmp/nrflo", "api-via-cli-system-*.md")
	if spfErr != nil {
		return nil, nil, fmt.Errorf("api-via-cli: create system prompt file: %w", spfErr)
	}
	if _, err := spf.WriteString(defaultAPISystemPrompt); err != nil {
		spf.Close()
		os.Remove(spf.Name())
		return nil, nil, fmt.Errorf("api-via-cli: write system prompt file: %w", err)
	}
	spf.Close()
	systemPromptOverrideFile := spf.Name()

	// Write the rendered prompt to a temp file for delivery via PTY stdin.
	pf, pfErr := os.CreateTemp("/tmp/nrflo", "api-via-cli-prompt-*.md")
	if pfErr != nil {
		os.Remove(systemPromptOverrideFile)
		return nil, nil, fmt.Errorf("api-via-cli: create prompt file: %w", pfErr)
	}
	if _, err := pf.WriteString(prompt); err != nil {
		pf.Close()
		os.Remove(pf.Name())
		os.Remove(systemPromptOverrideFile)
		return nil, nil, fmt.Errorf("api-via-cli: write prompt file: %w", err)
	}
	pf.Close()
	promptFile := pf.Name()

	// Build MCP config JSON pointing at the nrflo agent mcp bridge.
	mcpConfig, mcpErr := buildNrfloMCPConfig()
	if mcpErr != nil {
		os.Remove(systemPromptOverrideFile)
		os.Remove(promptFile)
		return nil, nil, fmt.Errorf("api-via-cli: build mcp config: %w", mcpErr)
	}

	opts := SpawnOptions{
		Model:                    claudeModel,
		MappedModel:              am.MappedModel,
		ReasoningEffort:          am.ReasoningEffort,
		SessionID:                sessionID,
		WorkDir:                  s.config.ProjectRoot,
		SettingsJSON:             s.config.ClaudeSettingsJSON,
		SystemPromptOverrideFile: systemPromptOverrideFile,
		MCPConfigJSON:            mcpConfig,
		NativeToolsCSV:           "Read",
		AllowedToolsCSV:          "mcp__nrflo__* Read",
		Env:                      s.buildCLIAgentEnv(ctx, req.ProjectID, wfiID, sessionID, spawnToken, effectiveThreshold, proc.maxContext, cliStageDir, extID, extCtx),
	}

	prep.executionMode = "cli_interactive"
	prep.cliName = "claude"
	prep.adapter = adapter
	prep.opts = opts
	prep.promptFile = promptFile
	prep.prompt = prompt
	prep.suffixFile = ""
	prep.systemPromptOverrideFile = systemPromptOverrideFile
	proc.env = opts.Env
	return proc, prep, nil
}
