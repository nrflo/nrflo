package spawner

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"be/internal/db"
	"be/internal/logger"
	"be/internal/ws"
)

// pool returns the shared connection pool, or nil if not configured.
func (s *Spawner) pool() *db.Pool {
	if s.config.Pool != nil {
		return s.config.Pool
	}
	return nil
}

// projectOrGlobalBool resolves a "true"/other boolean config value, preferring
// the project-scoped setting over the global one; defaults to false when unset.
func (s *Spawner) projectOrGlobalBool(projectID, key string) bool {
	pool := s.pool()
	if pool == nil {
		return false
	}
	if projectID != "" {
		if v, _ := pool.GetProjectConfig(projectID, key); v != "" {
			return v == "true"
		}
	}
	v, _ := pool.GetConfig(key)
	return v == "true"
}

// broadcast sends a WebSocket event via the in-process hub
func (s *Spawner) broadcast(eventType, projectID, ticketID, workflow string, data map[string]interface{}) {
	if s.config.WSHub == nil {
		logger.Warn(context.Background(), "broadcast skipped: no WebSocket hub configured")
		return
	}
	event := ws.NewEvent(eventType, projectID, ticketID, workflow, data)
	s.config.WSHub.Broadcast(event)
}

// logAgent logs an INFO-level agent message with the agent's trx and prefix.
func (s *Spawner) logAgent(proc *processInfo, msg string) {
	ctx := logger.WithTrx(context.Background(), proc.trx)
	logger.Info(ctx, s.formatPrefix(proc)+" "+msg)
}

// warnAgent logs a WARN-level agent message with the agent's trx and prefix.
func (s *Spawner) warnAgent(proc *processInfo, msg string) {
	ctx := logger.WithTrx(context.Background(), proc.trx)
	logger.Warn(ctx, s.formatPrefix(proc)+" "+msg)
}

// errorAgent logs an ERROR-level agent message with the agent's trx and prefix.
func (s *Spawner) errorAgent(proc *processInfo, msg string) {
	ctx := logger.WithTrx(context.Background(), proc.trx)
	logger.Error(ctx, s.formatPrefix(proc)+" "+msg)
}

// waitBeforeRetry waits for defaultFailRetryDelay before retrying a failed/timed-out agent.
// Returns true if the wait completed, false if the context was cancelled (should not retry).
// Broadcasts an agent.retry_waiting event before sleeping.
func (s *Spawner) waitBeforeRetry(ctx context.Context, proc *processInfo) bool {
	s.broadcast(ws.EventAgentRetryWaiting, proc.projectID, proc.ticketID, proc.workflowName, map[string]interface{}{
		"agent_type":         proc.agentType,
		"session_id":         proc.sessionID,
		"model_id":           proc.modelID,
		"delay_seconds":      int(defaultFailRetryDelay.Seconds()),
		"fail_restart_count": proc.failRestartCount,
		"max_fail_restarts":  proc.maxFailRestarts,
	})
	logger.Info(ctx, "waiting before fail-restart", "delay", defaultFailRetryDelay, "model", proc.modelID)
	select {
	case <-ctx.Done():
		return false
	case <-time.After(defaultFailRetryDelay):
		return true
	}
}

// startBackend selects an ExecutionBackend based purely on prep.executionMode:
//   - "api"            → apiBackend (in-process Anthropic runner)
//   - "script"         → scriptBackend (Python exec.Cmd)
//   - "cli_interactive" → cliInteractiveBackend (PTY)
//
// System agents (conflict-resolver, context-saver) flow through the same
// selector — their execution_mode is sourced from system_agent_definitions.execution_mode.
func (s *Spawner) startBackend(proc *processInfo, prep *prepResult) error {
	var backend ExecutionBackend
	switch prep.executionMode {
	case "api":
		backend = newAPIBackend(s)
	case "script":
		backend = newScriptBackend(s)
	case "cli_interactive":
		// codex 0.133 exposes no usable structured channel under PTY (no hooks,
		// no rollout JSONL), so codex agents are driven via `codex app-server`
		// JSON-RPC instead of the PTY/TUI. All other CLIs use the PTY backend.
		if prep.cliName == "codex" {
			backend = newCodexAppServerBackend(s)
		} else {
			backend = newCLIInteractiveBackend(prep.adapter, s, wrapPtyManager(s.config.PTYManager))
		}
	default:
		return fmt.Errorf("unknown execution_mode %q for agent %q", prep.executionMode, proc.agentType)
	}
	proc.backend = backend

	var effectiveMode string
	switch prep.executionMode {
	case "api":
		effectiveMode = "api"
	case "script":
		effectiveMode = "script"
	default:
		effectiveMode = "cli_interactive"
	}

	// Register sessionProc BEFORE backend.Start so a fast SessionStart hook
	// (or any other socket lookup keyed by sessionID) can find the proc the
	// moment Claude posts back, not after we've returned from Start.
	s.registerSessionProc(proc.sessionID, proc)

	// Insert the agent_sessions row BEFORE starting the child. Script agents
	// call c.context() as their very first action; writing the row only after
	// Start let that lookup race the INSERT and fail with "agent session not
	// found" under spawn contention (parallel layers + SQLite single-writer).
	// pid and the final spawn_command are unknown until Start returns and are
	// recorded by markAgentStarted below.
	rowCreated := s.createAgentSessionRow(proc.projectID, proc.ticketID, proc.workflowInstanceID,
		proc.agentType, proc.nodeID, proc.sessionID, proc.modelID, prep.phase,
		proc.spawnCommand, proc.prompt, proc.systemPrompt, proc.spawnToken, effectiveMode, 0)

	if err := backend.Start(context.Background(), proc, prep); err != nil {
		// Roll back only the row we inserted, so a failed spawn leaves no
		// orphaned "running" session. A pre-existing row (observer path) is
		// owned by its creator and left untouched.
		if rowCreated {
			s.deleteAgentSessionRow(proc.sessionID)
		}
		s.unregisterSessionProcs([]*processInfo{proc})
		return err
	}

	pid := proc.pid
	if proc.cmd != nil && proc.cmd.Process != nil {
		pid = proc.cmd.Process.Pid
	}
	s.markAgentStarted(proc.projectID, proc.ticketID, proc.workflowName,
		proc.agentID, proc.agentType, proc.modelID, proc.sessionID, prep.phase,
		proc.spawnCommand, pid, proc.restartThreshold)
	return nil
}

// HostEnvWithoutClaudeMarkers returns os.Environ() minus the nested-Claude
// markers (CLAUDECODE, CLAUDE_CODE_*). A child claude CLI that inherits a
// parent Claude Code session's markers treats itself as a nested child
// session — most damagingly CLAUDE_CODE_CHILD_SESSION, which makes claude
// ≥2.1 skip writing the project transcript JSONL that the console engine
// tailer and the resume-based context save read (verified against 2.1.211).
// Every spawned CLI env must start from this, never raw os.Environ().
func HostEnvWithoutClaudeMarkers() []string {
	hostEnv := os.Environ()
	out := make([]string, 0, len(hostEnv))
	for _, e := range hostEnv {
		if strings.HasPrefix(e, "CLAUDECODE=") || strings.HasPrefix(e, "CLAUDE_CODE_") {
			continue
		}
		out = append(out, e)
	}
	return out
}

func (s *Spawner) maxContextForModel(model string) int {
	if cfg, ok := s.config.ModelConfigs[model]; ok && cfg.ContextLength > 0 {
		return cfg.ContextLength
	}
	if model == "opus_4_6_1m" || model == "opus_4_7_1m" || model == "opus_4_8_1m" {
		return 1000000
	}
	return 200000
}

// cliForModel returns the CLI name for a model, checking DB config first.
func (s *Spawner) cliForModel(model string) string {
	if cfg, ok := s.config.ModelConfigs[model]; ok && cfg.CLIType != "" {
		return cfg.CLIType
	}
	return DefaultCLIForModel(model)
}

func parseModelID(modelID string) (cli, model string) {
	if modelID == "" || !strings.Contains(modelID, ":") {
		return "claude", modelID
	}
	parts := strings.SplitN(modelID, ":", 2)
	return parts[0], parts[1]
}
