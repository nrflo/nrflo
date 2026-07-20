package spawner

import (
	"context"
	"fmt"
	"os"
	"strings"

	"be/internal/logger"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/service"
)

// prepareAPIViaCLISpawn transforms an api-mode spawn into a cli_interactive spawn
// that drives Claude via PTY, using the nrflo agent mcp bridge to serve the
// api-mode tool registry to the Claude process. Called from the api branch of
// prepareSpawn when Config.APIViaCLI is true and the provider is "anthropic".
// Does NOT call BuildAPIProvider.
func (s *Spawner) prepareAPIViaCLISpawn(
	ctx context.Context,
	req SpawnRequest,
	wfiID, sessionID, spawnToken string,
	effectiveThreshold int,
	extID, extCtx string,
	prompt string,
	am ModelConfig,
	agentDef *model.AgentDefinition,
	proc *processInfo,
	prep *prepResult,
) (*processInfo, *prepResult, error) {
	adapter, err := GetCLIAdapter("claude")
	if err != nil {
		return nil, nil, fmt.Errorf("api-via-cli: %w", err)
	}

	// The hybrid deliberately uses the API mode's context, model, and efforts.
	proc.maxContext = am.APIContext

	// Build tool registry: honor the tools field but force the lifecycle
	// baseline — api-via-cli completes over the socket like a CLI agent.
	specs, handlers, toolEnv, regErr := s.buildAPIRegistry(req, wfiID, agentDef, proc, "", true, false, false)
	if regErr != nil {
		return nil, nil, regErr
	}

	// Substitute read_document with the path-returning variant: Claude has native
	// Read so it can access materialized artifacts by path directly.
	substituteReadDocumentPath(specs, handlers)

	proc.nudgeMax = 0
	proc.adapter = adapter
	// modelID must start "claude:" so BuildInteractiveSettingsJSON emits hooks+statusLine
	// and startBackend routes to the PTY backend (not codex app-server). The incoming
	// modelID is "<cliForModel>:<model>" (e.g. "claude:sonnet-5"), not "api:" — parse it.
	_, rawModel := parseModelID(proc.modelID)
	claudeModel := am.APIModel
	if claudeModel == "" {
		return nil, nil, fmt.Errorf("api-via-cli: model %q does not support api mode", rawModel)
	}
	// The Claude CLI selects its context window from the model STRING passed to
	// --model, not from proc.maxContext: bare "claude-opus-4-8" opens 200k, the
	// "[1m]" suffix opens 1M. The hybrid reports am.APIContext as the window, so
	// when the API window is 1M and exceeds what the bare string opens in the CLI
	// (am.CLIContext), the string must request 1M explicitly. sonnet-5 (API 1M,
	// CLI 1M) stays bare; opus-4-6/4-7/4-8 (API 1M > CLI 200k) gets the suffix.
	if am.APIContext >= 1_000_000 && am.APIContext > am.CLIContext && !strings.HasSuffix(claudeModel, "[1m]") {
		claudeModel += "[1m]"
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

	// Write the def-template-resolved system prompt (else defaultAPISystemPrompt,
	// byte-identical to today) to a temp file for --system-prompt-file.
	systemPromptBody := defaultAPISystemPrompt
	if agentDef != nil && agentDef.SystemTemplateID != "" {
		if rendered := s.expandInjectable(agentDef.SystemTemplateID, stdTemplateVars(req.AgentType, proc.nodeID, req.TicketID, req.ProjectID, req.WorkflowName, req.ParentSession, sessionID, proc.modelID, req.ExtraVars)); rendered != "" {
			systemPromptBody = rendered
		}
	}
	systemPromptBody = appendDelegationGuidance(ctx, s.pool(), systemPromptBody, specs, stdTemplateVars(req.AgentType, proc.nodeID, req.TicketID, req.ProjectID, req.WorkflowName, req.ParentSession, sessionID, proc.modelID, req.ExtraVars))
	spf, spfErr := createScratchTemp("api-via-cli-system-*.md")
	if spfErr != nil {
		return nil, nil, fmt.Errorf("api-via-cli: create system prompt file: %w", spfErr)
	}
	if _, err := spf.WriteString(systemPromptBody); err != nil {
		spf.Close()
		os.Remove(spf.Name())
		return nil, nil, fmt.Errorf("api-via-cli: write system prompt file: %w", err)
	}
	spf.Close()
	systemPromptOverrideFile := spf.Name()

	// Write the rendered prompt to a temp file for delivery via PTY stdin.
	pf, pfErr := createScratchTemp("api-via-cli-prompt-*.md")
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

	effort := s.resolveReasoningEffort(agentDef, req.AgentType, am.DefaultEffort)
	if err := service.ValidateEffortAllowed(effort, am.APIEfforts); err != nil {
		return nil, nil, fmt.Errorf("api-via-cli: %w", err)
	}

	opts := SpawnOptions{
		Model: claudeModel,
		// MappedModel is what backend_interactive passes to the CLI --model flag,
		// so the "[1m]" suffix must live here too — not just on Model.
		MappedModel:              claudeModel,
		ReasoningEffort:          effort,
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
