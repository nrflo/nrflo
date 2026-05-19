package spawner

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"

	"be/internal/logger"
	"be/internal/repo"
	"be/internal/service"
	"be/internal/ws"
)

// SpawnObserver starts a cli_interactive observer agent for the given session
// (already inserted into agent_sessions by ObserverService.Launch). It builds
// the env, mints the processInfo, starts the PTY backend, registers the
// interactive wait, and launches monitorAll in a goroutine. Returns as soon as
// the backend is started — the observer runs asynchronously.
func (s *Spawner) SpawnObserver(req service.ObserverSpawnRequest) error {
	ctx := context.Background()

	// Resolve which CLI adapter to use. Provider string maps to a CLI name;
	// default to "claude" if empty.
	cliName := req.Provider
	if cliName == "" {
		cliName = "claude"
	}
	adapter, err := GetCLIAdapter(cliName)
	if err != nil {
		// If the provider is not a known CLI name, treat it as a model name and
		// derive the CLI from it.
		cliName = s.cliForModel(req.Provider)
		adapter, err = GetCLIAdapter(cliName)
		if err != nil {
			return fmt.Errorf("spawn_observer: resolve cli adapter: %w", err)
		}
	}

	// Resolve model: use req.Model if set, otherwise fall back to "sonnet".
	model := req.Model
	if model == "" {
		model = "sonnet"
	}
	modelID := fmt.Sprintf("%s:%s", cliName, model)
	_, modelName := parseModelID(modelID)

	agentID := "obs-" + uuid.New().String()[:8]
	workDir := s.config.ProjectRoot
	if workDir == "" || workDir == "." {
		workDir = ""
	}

	// Assemble prompt: system context + separator + dynamic context + footer.
	var promptParts []string
	if req.SystemContext != "" {
		promptParts = append(promptParts, req.SystemContext)
	}
	if req.DynamicContext != "" {
		promptParts = append(promptParts, req.DynamicContext)
	}
	promptParts = append(promptParts, "---\n\nYou are an observer agent. Analyze the context above and provide insights.")
	fullPrompt := strings.Join(promptParts, "\n\n---\n\n")

	// Write prompt to temp file.
	promptFile, err := os.CreateTemp("/tmp/nrflo", fmt.Sprintf("observer-%s-*.md", req.Scope))
	if err != nil {
		return fmt.Errorf("spawn_observer: create prompt file: %w", err)
	}
	if _, err := promptFile.WriteString(fullPrompt); err != nil {
		os.Remove(promptFile.Name())
		return fmt.Errorf("spawn_observer: write prompt file: %w", err)
	}
	promptFile.Close()

	// Build env.
	env := append(filterEnv(os.Environ(), "CLAUDECODE"),
		fmt.Sprintf("NRFLO_PROJECT=%s", req.ProjectID),
		fmt.Sprintf("NRF_SESSION_ID=%s", req.SessionID),
		fmt.Sprintf("NRFLO_AGENT_TOKEN=%s", req.SpawnToken),
		"NRF_SPAWNED=1",
		"NRF_OBSERVER=1",
		fmt.Sprintf("NRF_OBSERVER_SCOPE=%s", req.Scope),
		fmt.Sprintf("NRF_PROJECT_ID=%s", req.ProjectID),
	)
	if req.WorkflowID != "" {
		env = append(env, fmt.Sprintf("NRF_WORKFLOW_ID=%s", req.WorkflowID))
	}
	env = append(env, s.config.ProjectEnv...)

	nudgeMax := defaultNudgeMax
	if s.config.NudgeMax > 0 {
		nudgeMax = s.config.NudgeMax
	}
	idleAfterMsg := defaultIdleAfterMessageTimeout
	if s.config.IdleAfterMessageTimeoutSec > 0 {
		idleAfterMsg = time.Duration(s.config.IdleAfterMessageTimeoutSec) * time.Second
	}
	idleStart := defaultIdleStartTimeout
	if s.config.IdleStartTimeoutSec > 0 {
		idleStart = time.Duration(s.config.IdleStartTimeoutSec) * time.Second
	}

	proc := &processInfo{
		agentID:                 agentID,
		agentType:               "_observer",
		modelID:                 modelID,
		sessionID:               req.SessionID,
		spawnToken:              req.SpawnToken,
		startTime:               s.config.Clock.Now(),
		timeout:                 40 * time.Minute,
		pendingMessages:         make([]repo.MessageEntry, 0),
		pendingTasks:            make(map[string]taskInfo),
		doneCh:                  make(chan struct{}),
		sessionStartCh:          make(chan struct{}),
		firstByteCh:             make(chan struct{}),
		lastMessagesFlush:       s.config.Clock.Now(),
		prompt:                  fullPrompt,
		projectID:               req.ProjectID,
		ticketID:                "",
		workflowName:            req.WorkflowID,
		workflowInstanceID:      "",
		restartThreshold:        defaultContextThreshold,
		lastMessageTime:         s.config.Clock.Now(),
		stallStartTimeout:       defaultStallStartTimeout,
		stallRunningTimeout:     defaultStallRunningTimeout,
		maxContext:              s.maxContextForModel(modelName),
		workDir:                 workDir,
		env:                     env,
		nudgeMax:                nudgeMax,
		idleAfterMessageTimeout: idleAfterMsg,
		idleStartTimeout:        idleStart,
		rateLimitConfig:         s.loadRateLimitConfig(req.ProjectID, adapter.Name()),
		adapter:                 adapter,
	}

	var mappedModel, reasoningEffort string
	if cfg, ok := s.config.ModelConfigs[model]; ok {
		mappedModel = cfg.MappedModel
		reasoningEffort = cfg.ReasoningEffort
	}

	prep := &prepResult{
		adapter:       adapter,
		cliName:       cliName,
		prompt:        fullPrompt,
		promptFile:    promptFile.Name(),
		phase:         "observer",
		executionMode: "cli_interactive",
		opts: SpawnOptions{
			Model:           model,
			SessionID:       req.SessionID,
			PromptFile:      promptFile.Name(),
			Prompt:          fullPrompt,
			WorkDir:         workDir,
			MappedModel:     mappedModel,
			ReasoningEffort: reasoningEffort,
			Env:             env,
		},
	}

	if err := s.startBackend(proc, prep); err != nil {
		os.Remove(promptFile.Name())
		return fmt.Errorf("spawn_observer: start backend: %w", err)
	}

	s.broadcast(ws.EventAgentStarted, req.ProjectID, "", req.WorkflowID, map[string]interface{}{
		"agent_id":          agentID,
		"agent_type":        "_observer",
		"model_id":          modelID,
		"session_id":        req.SessionID,
		"phase":             "observer",
		"restart_threshold": 0,
		"kind":              "observer",
	})
	s.broadcastGlobal()

	proc.trx = logger.NewTrx()

	// Update agent_sessions with spawn details now that we have them.
	if pool := s.pool(); pool != nil {
		sessionRepo := repo.NewAgentSessionRepo(pool, s.config.Clock)
		_ = sessionRepo.UpdateResult(req.SessionID, "", "")
	}

	// Register the interactive wait channel so PTY relay handlers can close it later.
	_ = s.RegisterInteractiveWait(req.SessionID)

	// Run monitoring in the background — observer runs asynchronously.
	spawnReq := SpawnRequest{
		ProjectID:    req.ProjectID,
		WorkflowName: req.WorkflowID,
	}
	go func() {
		if err := s.monitorAll(ctx, []*processInfo{proc}, spawnReq, "observer"); err != nil {
			logger.Warn(ctx, "observer monitorAll error", "session_id", req.SessionID, "error", err)
		}
		os.Remove(promptFile.Name())
	}()

	logger.Info(ctx, "observer spawned",
		"session_id", req.SessionID,
		"scope", req.Scope,
		"project_id", req.ProjectID,
		"model", modelID,
	)

	return nil
}
