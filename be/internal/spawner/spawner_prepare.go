package spawner

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"be/internal/id"
	"be/internal/logger"
	"be/internal/repo"
	"be/internal/service"
	"github.com/google/uuid"
)

// prepareSpawn does all CLI-agnostic prep work: session/agent IDs, agent-def
// lookup (timeouts, restart threshold, stall settings), template loading,
// prompt file creation, and SpawnOptions assembly. The returned processInfo
// has cmd left nil — startBackend wires up the chosen ExecutionBackend.
// The ctx's trx is threaded into NRF_TRX so socket-driven log lines from
// spawned agents share the workflow's trx.
func (s *Spawner) prepareSpawn(ctx context.Context, req SpawnRequest, modelID, phase, wfiID string) (*processInfo, *prepResult, error) {
	agentID := "spawn-" + uuid.New().String()[:8]
	sessionID := uuid.New().String()
	spawnToken := id.MintToken()

	// Parse modelID (cli:model format)
	cliName, model := parseModelID(modelID)
	if cliName == "" {
		cliName = s.cliForModel(model)
		modelID = fmt.Sprintf("%s:%s", cliName, model)
	}

	// Load agent definition early — execution_mode determines whether we
	// resolve a CLI adapter or skip CLI prep for api/script mode.
	agentDef := s.loadAgentDefinition(req.AgentType, req.ProjectID, req.WorkflowName)
	executionMode := "cli_interactive"
	if agentDef != nil && (agentDef.ExecutionMode == "api" || agentDef.ExecutionMode == "script" || agentDef.ExecutionMode == "cli_interactive") {
		executionMode = agentDef.ExecutionMode
	} else if agentDef == nil {
		if agentCfg, ok := s.config.Agents[req.AgentType]; ok && (agentCfg.ExecutionMode == "api" || agentCfg.ExecutionMode == "script" || agentCfg.ExecutionMode == "cli_interactive") {
			executionMode = agentCfg.ExecutionMode
		}
	}

	// Script mode: delegate to dedicated prep path (not gated by APIMode).
	if executionMode == "script" {
		return s.prepareScriptSpawn(ctx, req, phase, wfiID, agentID, sessionID, spawnToken, agentDef)
	}

	// Reject api-mode agents when api_mode_enabled is not set.
	if executionMode == "api" && !s.config.APIMode {
		return nil, nil, fmt.Errorf("api_mode_disabled")
	}

	// Get CLI adapter (api/script modes skip this — there is no CLI process)
	var adapter CLIAdapter
	if executionMode == "cli_interactive" {
		var err error
		adapter, err = GetCLIAdapter(cliName)
		if err != nil {
			return nil, nil, err
		}
	}

	// Get agent config for timeout lookup
	timeout := 40 // minutes
	if agentCfg, ok := s.config.Agents[req.AgentType]; ok {
		if agentCfg.Timeout > 0 {
			timeout = agentCfg.Timeout
		}
	}

	// Load agent definition to get per-agent restart threshold and fail restart limit
	effectiveThreshold, maxFailRestarts, stallStartTimeout, stallRunningTimeout, validationCommands :=
		s.resolveSpawnLimits(ctx, agentDef, req.AgentType)

	extID, extCtx, _ := s.fetchExternalRefs(req.ProjectID, req.TicketID, req.WorkflowName, wfiID)
	tmplVars := mergeExtraVars(req.ExtraVars, map[string]string{"EXTERNAL_ID": extID, "EXTERNAL_CONTEXT": extCtx})

	// Load agent template
	agentLayer := 0
	if agentDef != nil {
		agentLayer = agentDef.Layer
	}
	prompt, suffix, systemPromptOverride, err := s.loadTemplate(req.AgentType, req.TicketID, req.ProjectID, req.ParentSession, sessionID, req.WorkflowName, modelID, phase, req.WorkflowInstanceID, tmplVars, agentLayer)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load template: %w", err)
	}

	workDir := s.config.ProjectRoot
	if workDir == "" || workDir == "." {
		workDir = ""
	}

	_, modelName := parseModelID(modelID)
	proc := &processInfo{
		agentID:             agentID,
		agentType:           req.AgentType,
		nodeID:              phase,
		modelID:             modelID,
		sessionID:           sessionID,
		spawnToken:          spawnToken,
		startTime:           s.config.Clock.Now(),
		timeout:             time.Duration(timeout) * time.Minute,
		pendingMessages:     make([]repo.MessageEntry, 0),
		pendingTasks:        make(map[string]taskInfo),
		doneCh:              make(chan struct{}),
		sessionStartCh:      make(chan struct{}),
		firstByteCh:         make(chan struct{}),
		lastMessagesFlush:   s.config.Clock.Now(),
		prompt:              prompt,
		systemPrompt:        suffix,
		projectID:           req.ProjectID,
		ticketID:            req.TicketID,
		workflowName:        req.WorkflowName,
		workflowInstanceID:  wfiID,
		restartThreshold:    effectiveThreshold,
		maxFailRestarts:     maxFailRestarts,
		lastMessageTime:     s.config.Clock.Now(),
		stallStartTimeout:   stallStartTimeout,
		stallRunningTimeout: stallRunningTimeout,
		maxContext:          s.maxContextForModel(modelName),
		validationCommands:  validationCommands,
		workDir:             workDir,
	}
	proc.proactiveRestartThreshold = resolveProactiveRestartThreshold(agentDef, ProactiveRestartThresholdDefault(s.pool()))
	logger.Info(ctx, "agent spawned with validation commands", "agent", req.AgentType, "count", len(proc.validationCommands))

	// Populate idle/nudge fields only for cliInteractiveBackend agents.
	if executionMode == "cli_interactive" {
		nudgeMax := defaultNudgeMax
		if s.config.NudgeMax > 0 {
			nudgeMax = s.config.NudgeMax
		}
		proc.nudgeMax = nudgeMax

		idleAfterMsg := defaultIdleAfterMessageTimeout
		if s.config.IdleAfterMessageTimeoutSec > 0 {
			idleAfterMsg = time.Duration(s.config.IdleAfterMessageTimeoutSec) * time.Second
		}
		proc.idleAfterMessageTimeout = idleAfterMsg

		idleStart := defaultIdleStartTimeout
		if s.config.IdleStartTimeoutSec > 0 {
			idleStart = time.Duration(s.config.IdleStartTimeoutSec) * time.Second
		}
		proc.idleStartTimeout = idleStart
	}
	// nudgeMax = 0 (zero value) → disabled for non-interactive backends

	// Load rate-limit config and wire up adapter for cli_interactive mode.
	if executionMode == "cli_interactive" && adapter != nil {
		proc.rateLimitConfig = s.loadRateLimitConfig(req.ProjectID, adapter.Name())
		proc.adapter = adapter
	} else if executionMode == "api" {
		proc.rateLimitConfig = s.loadRateLimitConfig(req.ProjectID, "api")
	}

	prep := &prepResult{
		cliName: cliName, prompt: prompt, phase: phase, nodeID: phase,
		executionMode: executionMode, proactiveRestartThreshold: proc.proactiveRestartThreshold,
	}

	if executionMode == "api" {
		// API mode always requires a supported registry row.
		am, ok := s.config.ModelConfigs[model]
		if !ok {
			return nil, nil, fmt.Errorf("api mode: model %q not found in models", model)
		}
		if am.APIModel == "" {
			return nil, nil, fmt.Errorf("api mode: model %q does not support api mode", model)
		}

		// api-via-cli hybrid: route Anthropic api-models through the Claude CLI.
		if s.config.APIViaCLI && am.Provider == "anthropic" {
			return s.prepareAPIViaCLISpawn(ctx, req, wfiID, sessionID, spawnToken, effectiveThreshold, extID, extCtx, prompt, am, agentDef, proc, prep)
		}

		// Build the provider for this spawn. Fail fast on missing credentials.
		apiProv, provErr := s.config.BuildAPIProvider(ctx, am.Provider, req.ProjectID)
		if provErr != nil {
			return nil, nil, fmt.Errorf("api mode: %w", provErr)
		}
		prep.apiProvider = apiProv
		apiEffort := s.resolveReasoningEffort(agentDef, req.AgentType, am.DefaultEffort)
		if err := service.ValidateEffortAllowed(apiEffort, am.APIEfforts); err != nil {
			return nil, nil, fmt.Errorf("api mode: %w", err)
		}
		prep.apiReasoningEffort = apiEffort
		prep.apiCaptureThinking = s.projectOrGlobalBool(req.ProjectID, "capture_thinking_enabled")
		prep.apiContextBudget = resolveContextBudget(agentDef, contextConfigInt(s.pool(), "context_budget_default", 0))
		apiModelID := am.APIModel

		maxIter := defaultAPIMaxIterations
		if agentDef != nil && agentDef.APIMaxIterations != nil && *agentDef.APIMaxIterations > 0 {
			maxIter = *agentDef.APIMaxIterations
		} else if agentDef == nil {
			if agentCfg, ok := s.config.Agents[req.AgentType]; ok && agentCfg.APIMaxIterations != nil && *agentCfg.APIMaxIterations > 0 {
				maxIter = *agentCfg.APIMaxIterations
			}
		}

		maxTokens := defaultAPIMaxTokens
		if agentDef != nil && agentDef.APIMaxTokens != nil && *agentDef.APIMaxTokens > 0 {
			maxTokens = *agentDef.APIMaxTokens
		} else if agentDef == nil {
			if agentCfg, ok := s.config.Agents[req.AgentType]; ok && agentCfg.APIMaxTokens != nil && *agentCfg.APIMaxTokens > 0 {
				maxTokens = *agentCfg.APIMaxTokens
			}
		}
		maxCtx := am.APIContext // Registry context is authoritative; provider is fallback.
		if maxCtx <= 0 {
			maxCtx = apiProv.MaxContext(apiModelID)
		}
		proc.maxContext = maxCtx

		specs, handlers, toolEnv, regErr := s.buildAPIRegistry(req, wfiID, agentDef, proc, "", false, true, false)
		if regErr != nil {
			return nil, nil, regErr
		}

		prep.apiSystem = apiSystemPromptWithSuffix(ctx, s.pool(), stdTemplateVars(req.AgentType, phase, req.TicketID, req.ProjectID, req.WorkflowName, req.ParentSession, sessionID, modelID, tmplVars), suffix, defaultAPISystemPrompt, agentDefSystemTemplateID(agentDef))
		prep.apiInitialPrompt = prompt
		prep.apiTools = specs
		prep.apiHandlers = handlers
		prep.apiToolEnv = toolEnv
		prep.apiMaxIterations = maxIter
		prep.apiMaxTokens = maxTokens
		prep.apiDeadline = proc.startTime.Add(proc.timeout)
		prep.apiModelID = apiModelID
		prep.apiMaxContext = maxCtx
		return proc, prep, nil
	}

	// CLI mode: write prompt to temp file and assemble SpawnOptions.

	// Adapters without system-prompt-file support (Codex) get the override +
	// suffix prepended into the prompt body instead — see noSystemPromptFilePrefix.
	promptBody := prompt
	if prefix := noSystemPromptFilePrefix(suffix, systemPromptOverride, adapter); prefix != "" {
		promptBody = prefix + "\n\n" + prompt
	}

	// Backends that consume `prep.prompt` directly (cliInteractiveBackend
	// passing the body to PTY stdin or — for codex — to argv) must see the
	// suffix-prepended version too. prep.prompt was set earlier to the bare
	// body for parity with the API backend; overwrite now that promptBody
	// is final.
	prep.prompt = promptBody

	// Serve the nrflo agent commands as MCP tools (via the nrflo agent mcp
	// bridge) instead of the nrflo CLI. Built before temp files so an error
	// returns without leaking them. Claude consumes --mcp-config/--allowedTools
	// (set here); codex consumes the registry via a config.toml [mcp_servers]
	// table written by the codex app-server backend from proc.apiTools.
	var mcpConfigJSON, allowedToolsCSV string
	switch adapter.Name() {
	case "claude":
		var mcpErr error
		if mcpConfigJSON, allowedToolsCSV, mcpErr = s.configureClaudeMCPTools(req, wfiID, agentDef, proc, adapter); mcpErr != nil {
			return nil, nil, mcpErr
		}
	case "codex":
		if regErr := s.attachNrfloToolRegistry(req, wfiID, agentDef, proc, adapter); regErr != nil {
			return nil, nil, regErr
		}
	}

	filePrefix := req.TicketID
	if req.IsProjectScope() {
		filePrefix = "project-" + req.ProjectID
	}
	safePrefix := strings.ReplaceAll(filePrefix, "/", "_")
	safePrefix = strings.ReplaceAll(safePrefix, "\\", "_")
	promptFile, err := createScratchTemp(fmt.Sprintf("%s-%s-*.md", safePrefix, req.AgentType))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	if _, err := promptFile.WriteString(promptBody); err != nil {
		os.Remove(promptFile.Name())
		return nil, nil, fmt.Errorf("failed to write prompt: %w", err)
	}
	promptFile.Close()

	// Write the system-prompt-suffix and system-prompt-override injectables to
	// temp files (Claude only — adapter.SupportsSystemPromptFile()).
	suffixFilePath, systemPromptOverrideFilePath := writeSuffixAndOverrideFiles(suffix, systemPromptOverride, adapter)

	// Known models must support CLI mode; unknown values are raw CLI passthrough strings.
	cfg, modelFound := s.config.ModelConfigs[model]
	if modelFound && cfg.CLIModel == "" {
		return nil, nil, fmt.Errorf("cli mode: model %q does not support cli mode", model)
	}
	mappedModel := cfg.CLIModel
	fallbackModels := cfg.FallbackModels
	reasoningEffort := s.resolveReasoningEffort(agentDef, req.AgentType, cfg.DefaultEffort)
	if modelFound {
		if err := service.ValidateEffortAllowed(reasoningEffort, cfg.CLIEfforts); err != nil {
			return nil, nil, fmt.Errorf("cli mode: %w", err)
		}
	}

	cliStageDir, _ := EnsureStageDir(req.ProjectID, wfiID)
	if s.config.ArtifactSvc != nil {
		if pool := s.pool(); pool != nil {
			if storage, storageErr := s.config.ArtifactSvc.GetStorage(ctx, req.ProjectID); storageErr == nil {
				if _, matErr := MaterializeAll(ctx, wfiID, req.ProjectID, repo.NewArtifactRepo(pool, s.config.Clock), storage); matErr != nil {
					logger.Warn(ctx, "artifact pre-materialize failed during cli spawn", "error", matErr)
				}
			} else {
				logger.Warn(ctx, "artifact storage unavailable during cli spawn", "error", storageErr)
			}
		}
	}

	nativeToolsCSV, sandbox := nativeSpawnFields(agentDef, adapter.Name())
	opts := SpawnOptions{
		Model:                    model,
		SessionID:                sessionID,
		PromptFile:               promptFile.Name(),
		Prompt:                   promptBody,
		WorkDir:                  workDir,
		MappedModel:              mappedModel,
		ReasoningEffort:          reasoningEffort,
		FallbackModels:           fallbackModels,
		SettingsJSON:             s.config.ClaudeSettingsJSON,
		SystemPromptFile:         suffixFilePath,
		SystemPromptOverrideFile: systemPromptOverrideFilePath,
		MCPConfigJSON:            mcpConfigJSON,
		AllowedToolsCSV:          allowedToolsCSV,
		NativeToolsCSV:           nativeToolsCSV,
		Sandbox:                  sandbox,
		Env:                      s.buildCLIAgentEnv(ctx, req.ProjectID, wfiID, sessionID, spawnToken, effectiveThreshold, proc.maxContext, cliStageDir, extID, extCtx),
	}

	prep.adapter = adapter
	prep.opts = opts
	prep.promptFile = promptFile.Name()
	prep.suffixFile = suffixFilePath
	prep.systemPromptOverrideFile = systemPromptOverrideFilePath
	proc.env = opts.Env
	return proc, prep, nil
}
