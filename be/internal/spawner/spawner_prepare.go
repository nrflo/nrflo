package spawner

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"

	"be/internal/logger"
	"be/internal/repo"
	"be/internal/spawner/apirun"
	"be/internal/spawner/apirun/tools_builtin"
	"be/internal/spawner/apirun/tools_http"
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
	spawnToken := MintSpawnToken()

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
	effectiveThreshold := defaultContextThreshold
	maxFailRestarts := 0
	if agentDef != nil && agentDef.RestartThreshold != nil {
		effectiveThreshold = *agentDef.RestartThreshold
	}
	if agentDef != nil && agentDef.MaxFailRestarts != nil {
		maxFailRestarts = *agentDef.MaxFailRestarts
	}
	stallStartTimeout := defaultStallStartTimeout
	stallRunningTimeout := defaultStallRunningTimeout
	if agentDef != nil && agentDef.StallStartTimeoutSec != nil {
		if *agentDef.StallStartTimeoutSec == 0 {
			stallStartTimeout = 0
		} else {
			stallStartTimeout = time.Duration(*agentDef.StallStartTimeoutSec) * time.Second
		}
	} else if s.config.GlobalStallStartTimeout != nil {
		if *s.config.GlobalStallStartTimeout == 0 {
			stallStartTimeout = 0
		} else {
			stallStartTimeout = time.Duration(*s.config.GlobalStallStartTimeout) * time.Second
		}
	}
	if agentDef != nil && agentDef.StallRunningTimeoutSec != nil {
		if *agentDef.StallRunningTimeoutSec == 0 {
			stallRunningTimeout = 0
		} else {
			stallRunningTimeout = time.Duration(*agentDef.StallRunningTimeoutSec) * time.Second
		}
	} else if s.config.GlobalStallRunningTimeout != nil {
		if *s.config.GlobalStallRunningTimeout == 0 {
			stallRunningTimeout = 0
		} else {
			stallRunningTimeout = time.Duration(*s.config.GlobalStallRunningTimeout) * time.Second
		}
	}

	var validationCommands []string
	if agentDef != nil && agentDef.ValidationCommands != "" {
		if jsonErr := json.Unmarshal([]byte(agentDef.ValidationCommands), &validationCommands); jsonErr != nil {
			logger.Warn(ctx, "failed to parse validation_commands", "agent", req.AgentType, "error", jsonErr)
			validationCommands = nil
		}
	}

	extID, extCtx := s.fetchExternalRefs(req.ProjectID, req.TicketID, req.WorkflowName, wfiID)
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
		cliName:       cliName,
		prompt:        prompt,
		phase:         phase,
		executionMode: executionMode,
	}

	if executionMode == "api" {
		// Look up api_models row for this model. Fail fast if not configured.
		am, ok := s.config.APIModelConfigs[model]
		if !ok {
			return nil, nil, fmt.Errorf("api mode: model %q not found in api_models", model)
		}

		// Build the provider for this spawn. Fail fast on missing credentials.
		apiProv, provErr := s.config.BuildAPIProvider(ctx, am.Provider, req.ProjectID)
		if provErr != nil {
			return nil, nil, fmt.Errorf("api mode: %w", provErr)
		}
		prep.apiProvider = apiProv
		prep.apiReasoningEffort = am.ReasoningEffort

		// Resolve mapped model name from the api_models row.
		apiModelID := model
		if am.MappedModel != "" {
			apiModelID = am.MappedModel
		}

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
		maxCtx := am.ContextLength
		if pmc := apiProv.MaxContext(apiModelID); pmc > 0 {
			maxCtx = pmc
		}
		proc.maxContext = maxCtx

		// Resolve per-agent tool registry from the CSV. Empty CSV ⇒ text-only.
		toolsCSV := ""
		if agentDef != nil {
			toolsCSV = agentDef.Tools
		} else if agentCfg, ok := s.config.Agents[req.AgentType]; ok {
			toolsCSV = agentCfg.Tools
		}
		httpDefs, defsErr := s.loadAPIHTTPToolDefs(req.ProjectID, req.WorkflowName)
		if defsErr != nil {
			return nil, nil, fmt.Errorf("api mode: load tool defs: %w", defsErr)
		}

		// Load python tool handlers for this project.
		pythonHandlers, _ := s.loadProjectPythonTools(req.ProjectID, proc.sessionID)

		specs, handlers, regErr := apirun.ResolveRegistry(toolsCSV, tools_builtin.Builtins(), pythonHandlers, httpDefs, tools_http.New(nil))
		if regErr != nil {
			return nil, nil, fmt.Errorf("api mode: %w", regErr)
		}

		// Recursion guard: consultant agents may not call consult themselves.
		if agentDef != nil && agentDef.Consultant {
			delete(handlers, "consult")
			filtered := specs[:0]
			for _, spec := range specs {
				if spec.Name != "consult" {
					filtered = append(filtered, spec)
				}
			}
			specs = filtered
		}

		// The system-prompt-suffix is a CLI completion contract ("run `nrflo
		// agent finished` via the Bash tool") that is meaningless in api mode:
		// there is no Bash tool here. Completion is signaled by the
		// agent_finished/agent_fail builtin tools (or implicit PASS on end_turn).
		prep.apiSystem = defaultAPISystemPrompt
		prep.apiInitialPrompt = prompt
		prep.apiTools = specs
		prep.apiHandlers = handlers
		prep.apiToolEnv = apirun.ToolEnv{
			Pool:               s.config.Pool,
			WSHub:              s.config.WSHub,
			Clock:              s.config.Clock,
			DispatchRepo:       s.config.DispatchRepo,
			SessionID:          proc.sessionID,
			AgentID:            proc.agentID,
			AgentType:          req.AgentType,
			ProjectID:          req.ProjectID,
			TicketID:           req.TicketID,
			WorkflowName:       req.WorkflowName,
			WorkflowInstanceID: wfiID,
			Findings:           s.config.FindingsSvc,
			ProjectFindings:    s.config.ProjectFindingsSvc,
			Agent:              s.config.AgentSvcReal,
			Workflow:           s.config.WorkflowSvc,
			ArtifactSvc:        s.config.ArtifactSvc,
			WorkflowControl:    s.config.WorkflowControl,
			Consultant:         s,
		}
		prep.apiMaxIterations = maxIter
		prep.apiMaxTokens = maxTokens
		prep.apiDeadline = proc.startTime.Add(proc.timeout)
		prep.apiModelID = apiModelID
		prep.apiMaxContext = maxCtx
		return proc, prep, nil
	}

	// CLI mode: write prompt to temp file and assemble SpawnOptions.

	// For adapters without system-prompt-file support (Codex), prepend
	// the suffix directly into the prompt body so it is delivered via the prompt file.
	promptBody := prompt
	if suffix != "" && !adapter.SupportsSystemPromptFile() {
		promptBody = suffix + "\n\n" + prompt
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
	promptFile, err := os.CreateTemp("/tmp/nrflo", fmt.Sprintf("%s-%s-*.md", safePrefix, req.AgentType))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	if _, err := promptFile.WriteString(promptBody); err != nil {
		os.Remove(promptFile.Name())
		return nil, nil, fmt.Errorf("failed to write prompt: %w", err)
	}
	promptFile.Close()

	// For adapters that support --append-system-prompt-file (Claude), write
	// the suffix to a separate temp file so Claude appends it to its system prompt.
	var suffixFilePath string
	if suffix != "" && adapter.SupportsSystemPromptFile() {
		sf, sfErr := os.CreateTemp("/tmp/nrflo", "system-suffix-*.md")
		if sfErr != nil {
			logger.Warn(context.Background(), "failed to create suffix temp file", "error", sfErr)
		} else {
			if _, sfErr = sf.WriteString(suffix); sfErr != nil {
				sf.Close()
				os.Remove(sf.Name())
				logger.Warn(context.Background(), "failed to write suffix temp file", "error", sfErr)
			} else {
				sf.Close()
				suffixFilePath = sf.Name()
			}
		}
	}

	// For Claude with the system-prompt override on, write the system-prompt injectable to a
	// temp file for --system-prompt-file (overrides the default system prompt).
	var systemPromptOverrideFilePath string
	if systemPromptOverride != "" && adapter.SupportsSystemPromptFile() {
		of, ofErr := os.CreateTemp("/tmp/nrflo", "system-prompt-*.md")
		if ofErr != nil {
			logger.Warn(context.Background(), "failed to create system-prompt override temp file", "error", ofErr)
		} else {
			if _, ofErr = of.WriteString(systemPromptOverride); ofErr != nil {
				of.Close()
				os.Remove(of.Name())
				logger.Warn(context.Background(), "failed to write system-prompt override temp file", "error", ofErr)
			} else {
				of.Close()
				systemPromptOverrideFilePath = of.Name()
			}
		}
	}

	// DB-sourced mapped model + reasoning effort
	var mappedModel, reasoningEffort string
	if cfg, ok := s.config.ModelConfigs[model]; ok {
		mappedModel = cfg.MappedModel
		reasoningEffort = cfg.ReasoningEffort
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

	opts := SpawnOptions{
		Model:                    model,
		SessionID:                sessionID,
		PromptFile:               promptFile.Name(),
		Prompt:                   promptBody,
		WorkDir:                  workDir,
		MappedModel:              mappedModel,
		ReasoningEffort:          reasoningEffort,
		SettingsJSON:             s.config.ClaudeSettingsJSON,
		SystemPromptFile:         suffixFilePath,
		SystemPromptOverrideFile: systemPromptOverrideFilePath,
		Env: s.buildCLIAgentEnv(ctx, req.ProjectID, wfiID, sessionID, spawnToken, effectiveThreshold, proc.maxContext, cliStageDir, extID, extCtx),
	}

	prep.adapter = adapter
	prep.opts = opts
	prep.promptFile = promptFile.Name()
	prep.suffixFile = suffixFilePath
	prep.systemPromptOverrideFile = systemPromptOverrideFilePath
	proc.env = opts.Env
	return proc, prep, nil
}
