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
	// Tier-fallback override wins over both agentDef and config.Agents so a
	// relaunch under the next chain entry can force cross-mode execution.
	if req.ExecutionModeOverride != "" {
		executionMode = req.ExecutionModeOverride
	}

	// Script mode: delegate to dedicated prep path (not gated by APIMode).
	if executionMode == "script" {
		return s.prepareScriptSpawn(ctx, req, phase, wfiID, agentID, sessionID, spawnToken, agentDef)
	}

	// Reject api-mode agents when api_mode_enabled is not set. This is a
	// build-time provider-construct failure (not structural), so a chain
	// carrying a non-api fallback entry can advance past it.
	if executionMode == "api" && !s.config.APIMode {
		return nil, nil, wrapProviderBuildErr(fmt.Errorf("api_mode_disabled"))
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
		return s.prepareAPIModeSpawn(ctx, req, model, modelID, phase, wfiID, sessionID, spawnToken, effectiveThreshold, extID, extCtx, prompt, suffix, tmplVars, agentDef, proc, prep)
	}

	// CLI mode: write prompt to temp file and assemble SpawnOptions.

	// Serve the nrflo agent commands as MCP tools (via the nrflo agent mcp
	// bridge) instead of the nrflo CLI. Built before temp files so an error
	// returns without leaking them, and before the promptBody/suffix assembly
	// below so proc.apiTools is populated for appendDelegationGuidance. Claude
	// consumes --mcp-config/--allowedTools (set here); codex consumes the
	// registry via a config.toml [mcp_servers] table written by the codex
	// app-server backend from proc.apiTools.
	mcpConfigJSON, allowedToolsCSV, regErr := s.configureCLIToolRegistry(req, wfiID, agentDef, proc, adapter)
	if regErr != nil {
		return nil, nil, regErr
	}

	suffix = appendDelegationGuidance(ctx, s.pool(), suffix, proc.apiTools, stdTemplateVars(req.AgentType, phase, req.TicketID, req.ProjectID, req.WorkflowName, req.ParentSession, sessionID, modelID, tmplVars))
	proc.systemPrompt = suffix

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
		return nil, nil, wrapProviderBuildErr(fmt.Errorf("cli mode: model %q does not support cli mode", model))
	}
	mappedModel := cfg.CLIModel
	fallbackModels := cfg.FallbackModels
	proc.resolvedEffort = s.resolveReasoningEffort(agentDef, req.AgentType, cfg.DefaultEffort)
	if req.ReasoningEffortOverride != "" {
		proc.resolvedEffort = req.ReasoningEffortOverride
	}
	if modelFound {
		if err := service.ValidateEffortAllowed(proc.resolvedEffort, cfg.CLIEfforts); err != nil {
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
		ReasoningEffort:          proc.resolvedEffort,
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
