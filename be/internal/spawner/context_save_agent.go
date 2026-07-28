package spawner

import (
	"context"
	"database/sql"
	"errors"

	"be/internal/foldfmt"
	"be/internal/logger"
	"be/internal/repo"
	"be/internal/service"
)

// The context-saver agent spawn itself. The decision of WHETHER to spawn it
// lives in context_save.go — this file is only the how, kept separate so the
// kill-time policy stays readable next to the refinery-digest short-circuit.

// contextSaverModel returns the model + reasoning effort the context-saver
// should run with, inherited from the dying agent's own resolved spawn state
// — proc.modelID (bare model parsed out of the "cli:model" form actually
// spawned, reflecting LowConsumptionMode overrides) and proc.resolvedEffort
// (the resolveReasoningEffort winner actually used for this spawn, reflecting
// per-project/workflow agentDef overrides) — mirroring the inline api
// compaction at apirun/conversation_compact.go:118, which sets
// Model/ReasoningEffort from the running agent's own Config. Falls back to
// defModel (the context-saver system agent def's model) when the dying
// agent's model is empty/unresolvable.
func (s *Spawner) contextSaverModel(proc *processInfo, defModel string) (string, *string) {
	_, bareModel := parseModelID(proc.modelID)
	model := defModel
	if bareModel != "" {
		model = bareModel
	}
	var effort *string
	if e := proc.resolvedEffort; e != "" {
		effort = &e
	}
	return model, effort
}

// spawnContextSaver loads the context-saver system agent and spawns it to save
// the original agent's message history, running it on the dying agent's own
// inherited model + reasoning effort (falling back to the system agent def's
// model when unresolvable). Returns true if the saver ran (regardless
// of whether it actually wrote findings). On any error, logs a warning and returns false.
func (s *Spawner) spawnContextSaver(ctx context.Context, proc *processInfo, req SpawnRequest) bool {
	pool := s.pool()
	if pool == nil {
		logger.Warn(ctx, "no database pool for context saver", "session_id", proc.sessionID)
		return false
	}

	// Determine backend name for saver selection (api backend uses context-saver-api variant)
	backendName := "cli"
	if proc.backend != nil {
		backendName = proc.backend.Name()
	}

	// Load system agent definition, preferring a backend-specific variant.
	svc := service.NewSystemAgentDefinitionService(pool, s.config.Clock, service.NewModelService(pool, s.config.Clock))
	sysDef, err := svc.GetForBackend("context-saver", backendName)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			logger.Warn(ctx, "context-saver system agent not found, relaunching without save", "err", err, "session_id", proc.sessionID)
			return false
		}
		// No backend-specific variant — fall back to the default CLI context-saver.
		logger.Warn(ctx, "no context-saver variant for backend, falling back to default", "backend", backendName, "session_id", proc.sessionID, "err", err)
		sysDef, err = svc.Get("context-saver")
		if err != nil {
			logger.Warn(ctx, "context-saver system agent not found, relaunching without save", "err", err, "session_id", proc.sessionID)
			return false
		}
	}

	// Fetch message history
	msgRepo := repo.NewAgentMessageRepo(pool, s.config.Clock)
	messages, err := msgRepo.GetBySession(proc.sessionID)
	if err != nil {
		logger.Warn(ctx, "failed to fetch agent messages for context save", "err", err, "session_id", proc.sessionID)
		return false
	}
	if len(messages) == 0 {
		logger.Warn(ctx, "no messages to save for context saver", "session_id", proc.sessionID)
		return false
	}

	formatted := foldfmt.JoinTail(messages, maxMessageChars)

	defModel := sysDef.Model
	chain, chainErr := svc.ResolveAgentChain(sysDef)
	if chainErr == nil && len(chain) > 0 {
		defModel = chain[0].ModelID
	} else if chainErr != nil {
		logger.Warn(ctx, "context-saver: resolve agent chain failed, using def model fallback", "err", chainErr, "session_id", proc.sessionID)
	}
	saverModel, saverEffort := s.contextSaverModel(proc, defModel)

	// Construct one-off spawner (conflict-resolver pattern), forwarding API-mode
	// dependencies so a context-saver-api variant can run via the in-process runner.
	// PTYManager is forwarded so a context-saver with execution_mode='cli_interactive'
	// can spawn inside a PTY when its system_agent_definitions row calls for it.
	saverRegister, saverUnregister := s.childSessionHooks(nil)
	sp := New(Config{
		Workflows: map[string]WorkflowDef{
			"_context_save": {
				Phases: []PhaseDef{{NodeID: "context-saver", Agent: "context-saver", Layer: 0}},
			},
		},
		Agents: map[string]AgentConfig{
			"context-saver": {
				Model:            saverModel,
				ReasoningEffort:  saverEffort,
				Timeout:          sysDef.Timeout,
				ExecutionMode:    sysDef.ExecutionMode,
				Tools:            sysDef.Tools,
				APIMaxIterations: sysDef.APIMaxIterations,
				APIMaxTokens:     sysDef.APIMaxTokens,
				Chain:            chain,
			},
		},
		DataPath:           s.config.DataPath,
		ProjectRoot:        s.config.ProjectRoot,
		WSHub:              s.config.WSHub,
		Pool:               pool,
		Clock:              s.config.Clock,
		ClaudeSettingsJSON: s.config.ClaudeSettingsJSON,
		ModelConfigs:       s.config.ModelConfigs,
		ErrorSvc:           s.config.ErrorSvc,
		BuildAPIProvider:   s.config.BuildAPIProvider,
		AgentSvc:           s.config.AgentSvc,
		FindingsSvc:        s.config.FindingsSvc,
		ProjectFindingsSvc: s.config.ProjectFindingsSvc,
		AgentSvcReal:       s.config.AgentSvcReal,
		WorkflowSvc:        s.config.WorkflowSvc,
		TicketSvc:          s.config.TicketSvc,
		PTYManager:         s.config.PTYManager,
		ProjectEnv:         s.config.ProjectEnv,
		APIMode:            true,
		// A cli_interactive context-saver is heartbeat-driven like any other
		// PTY agent; without the forwarded hooks its record-event bumps reach
		// no proc and it start-stalls at 2 min while writing a valid handoff.
		OnSessionRegister:   saverRegister,
		OnSessionUnregister: saverUnregister,
	})

	saveCtx, cancel := context.WithTimeout(ctx, contextSaveTimeout)
	defer cancel()

	spawnErr := sp.Spawn(saveCtx, SpawnRequest{
		AgentType:          "context-saver",
		NodeID:             "context-saver",
		TicketID:           req.TicketID,
		ProjectID:          req.ProjectID,
		WorkflowName:       "_context_save",
		WorkflowInstanceID: req.WorkflowInstanceID,
		ScopeType:          req.ScopeType,
		ExtraVars: map[string]string{
			"AGENT_TYPE":        proc.agentType,
			"AGENT_MESSAGES":    formatted,
			"TARGET_SESSION_ID": proc.sessionID,
			"WORKFLOW":          req.WorkflowName,
			"TICKET_ID":         req.TicketID,
		},
	})
	sp.Close()

	if spawnErr != nil {
		logger.Warn(ctx, "context-saver agent failed", "err", spawnErr, "session_id", proc.sessionID)
		return false
	}

	return true
}
