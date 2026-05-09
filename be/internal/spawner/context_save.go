package spawner

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"syscall"
	"time"

	"be/internal/logger"
	"be/internal/repo"
	"be/internal/service"
	"be/internal/ws"
)

const (
	contextSaveTimeout = 3 * time.Minute
	killGracePeriod    = 5 * time.Second
	maxMessageChars    = 120000
)

// initiateContextSave handles the low-context save flow:
// 1. Kill the running agent
// 2. Flush messages
// 3. Delegate to either system-agent or resume-based save depending on config
//
// processDoneCh is the original process's done channel (closed by the wait goroutine).
// completeCh is the replacement channel; closed when the full flow finishes, signaling monitorAll.
func (s *Spawner) initiateContextSave(ctx context.Context, proc *processInfo, req SpawnRequest, processDoneCh, completeCh chan struct{}) {
	defer close(completeCh)

	logger.Warn(ctx, "low context detected", "context_left", proc.contextLeft, "session_id", proc.sessionID)

	// 1. Kill the running agent: SIGTERM → wait → SIGKILL
	proc.backend.Kill(ctx, proc, syscall.SIGTERM)
	select {
	case <-processDoneCh:
		// Original process exited
	case <-time.After(killGracePeriod):
		proc.backend.Kill(ctx, proc, syscall.SIGKILL)
		<-processDoneCh
	}

	// 2. Flush messages from the killed process
	s.saveMessages(proc)

	// 3. Save context via configured method.
	if s.shouldUseAgentSave(proc) {
		s.contextSaveViaAgent(ctx, proc, req)
	} else {
		s.contextSaveViaResume(ctx, proc, req)
	}
}

// shouldUseAgentSave decides whether the system-agent context-saver path is
// required, regardless of the global `context_save_via_agent` setting.
//
// Forced cases:
//  1. Global setting enabled — user opt-in.
//  2. API-mode backend — `contextSaveViaResume` requires `--resume <session>`,
//     which doesn't apply to an in-process API run.
//  3. Adapter doesn't support resume — codex (and any future non-resumable CLI)
//     would otherwise short-circuit `contextSaveViaResume` with a warning and
//     relaunch blind, dropping all carryover. The agent-save path works for
//     every CLI because it reads `agent_messages` rather than resuming the
//     session.
//
// Costs one haiku context-saver invocation per low-context event for non-claude
// CLIs even when the user hasn't opted into agent-save. That's the right
// tradeoff: the previous default was silent data loss.
func (s *Spawner) shouldUseAgentSave(proc *processInfo) bool {
	if s.config.ContextSaveViaAgent {
		return true
	}
	if proc.backend != nil && proc.backend.Name() == "api" {
		return true
	}
	cliName, _ := parseModelID(proc.modelID)
	if cliName == "" {
		return false
	}
	adapter, err := GetCLIAdapter(cliName)
	if err != nil {
		return false
	}
	return !adapter.SupportsResume()
}

// contextSaveViaAgent uses a system agent (haiku) to summarize the killed agent's
// message history and save to_resume findings. Works for all CLI types.
func (s *Spawner) contextSaveViaAgent(ctx context.Context, proc *processInfo, req SpawnRequest) {
	// Broadcast context_saving event
	if s.config.WSHub != nil {
		s.config.WSHub.Broadcast(ws.NewEvent(ws.EventAgentContextSaving, req.ProjectID, req.TicketID, req.WorkflowName, map[string]interface{}{
			"session_id": proc.sessionID,
			"agent_type": proc.agentType,
		}))
	}

	// Spawn context-saver system agent
	saved := s.spawnContextSaver(ctx, proc, req)

	// Check if to_resume findings were actually saved
	findingsSaved := s.checkToResumeFindings(ctx, proc)
	if saved && !findingsSaved {
		logger.Warn(ctx, "context-saver completed but to_resume findings not saved, previous data will be empty on relaunch", "session_id", proc.sessionID)
	}

	// Register stop
	s.registerAgentStopWithReason(req.ProjectID, req.TicketID, req.WorkflowName,
		proc.sessionID, proc.agentID, "continue", "low_context", proc.modelID)

	proc.finalStatus = "CONTINUE"
	logger.Info(ctx, "context save flow complete, relaunching", "findings_saved", findingsSaved, "session_id", proc.sessionID)
}

// spawnContextSaver loads the context-saver system agent and spawns it to save
// the original agent's message history. Returns true if the saver ran (regardless
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
	svc := service.NewSystemAgentDefinitionService(pool, s.config.Clock)
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

	formatted := formatMessagesForSave(messages, maxMessageChars)

	// Construct one-off spawner (conflict-resolver pattern), forwarding API-mode
	// dependencies so a context-saver-api variant can run via the in-process runner.
	// InteractiveCLIMode and PTYManager are forwarded so the CLI context-saver also
	// runs interactively when the toggle is on.
	sp := New(Config{
		Workflows: map[string]WorkflowDef{
			"_context_save": {
				Phases: []PhaseDef{{ID: "context-saver", Agent: "context-saver", Layer: 0}},
			},
		},
		Agents: map[string]AgentConfig{
			"context-saver": {
				Model:            sysDef.Model,
				Timeout:          sysDef.Timeout,
				ExecutionMode:    sysDef.ExecutionMode,
				Tools:            sysDef.Tools,
				APIMaxIterations: sysDef.APIMaxIterations,
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
		Provider:           s.config.Provider,
		AgentSvc:           s.config.AgentSvc,
		FindingsSvc:        s.config.FindingsSvc,
		ProjectFindingsSvc: s.config.ProjectFindingsSvc,
		AgentSvcReal:       s.config.AgentSvcReal,
		WorkflowSvc:        s.config.WorkflowSvc,
		APICredentialRepo:  s.config.APICredentialRepo,
		ToolDefRepo:        s.config.ToolDefRepo,
		InteractiveCLIMode: s.config.InteractiveCLIMode,
		PTYManager:         s.config.PTYManager,
		ProjectEnv:         s.config.ProjectEnv,
	})

	saveCtx, cancel := context.WithTimeout(ctx, contextSaveTimeout)
	defer cancel()

	spawnErr := sp.Spawn(saveCtx, SpawnRequest{
		AgentType:          "context-saver",
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
			"TICKET_ID":        req.TicketID,
		},
	})
	sp.Close()

	if spawnErr != nil {
		logger.Warn(ctx, "context-saver agent failed", "err", spawnErr, "session_id", proc.sessionID)
		return false
	}

	return true
}

// checkToResumeFindings checks whether the session has to_resume findings after context save.
// Returns true if the to_resume key was found in the session's findings.
func (s *Spawner) checkToResumeFindings(ctx context.Context, proc *processInfo) bool {
	pool := s.pool()
	if pool == nil {
		logger.Error(ctx, "no database pool for findings check", "session_id", proc.sessionID)
		return false
	}

	var findingsRaw sql.NullString
	err := pool.QueryRow("SELECT findings FROM agent_sessions WHERE id = ?", proc.sessionID).Scan(&findingsRaw)
	if err != nil {
		logger.Error(ctx, "failed to query findings", "err", err, "session_id", proc.sessionID)
		return false
	}

	if !findingsRaw.Valid || findingsRaw.String == "" || findingsRaw.String == "{}" {
		logger.Warn(ctx, "no findings saved by context-saver agent", "session_id", proc.sessionID)
		return false
	}

	var findings map[string]interface{}
	if json.Unmarshal([]byte(findingsRaw.String), &findings) != nil {
		logger.Warn(ctx, "failed to parse findings JSON", "session_id", proc.sessionID)
		return false
	}

	toResume, ok := findings["to_resume"]
	if !ok {
		logger.Warn(ctx, "findings saved but to_resume key missing", "keys_count", len(findings), "session_id", proc.sessionID)
		return false
	}

	str, isStr := toResume.(string)
	if !isStr || str == "" {
		logger.Warn(ctx, "to_resume key present but empty or non-string", "session_id", proc.sessionID)
		return false
	}

	logger.Info(ctx, "to_resume findings saved", "bytes", len(str), "session_id", proc.sessionID)
	return true
}

// formatMessagesForSave joins messages with newlines. If total length exceeds
// maxChars, keeps the LAST N messages (most recent work is most relevant) and
// prepends a truncation header.
func formatMessagesForSave(messages []string, maxChars int) string {
	joined := strings.Join(messages, "\n")
	if len(joined) <= maxChars {
		return joined
	}

	// Keep tail messages that fit within maxChars
	var kept []string
	total := 0
	for i := len(messages) - 1; i >= 0; i-- {
		msgLen := len(messages[i])
		if total > 0 {
			msgLen++ // account for newline separator
		}
		if total+msgLen > maxChars {
			break
		}
		total += msgLen
		kept = append(kept, messages[i])
	}

	// Reverse to restore original order
	for i, j := 0, len(kept)-1; i < j; i, j = i+1, j-1 {
		kept[i], kept[j] = kept[j], kept[i]
	}

	header := fmt.Sprintf("[truncated: showing last %d of %d messages]", len(kept), len(messages))
	return header + "\n" + strings.Join(kept, "\n")
}
