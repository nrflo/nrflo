package spawner

import (
	"context"
	"fmt"
	"os"
	"strings"
	"syscall"
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

// broadcastSessionCost emits a project-scoped EventSessionCostUpdated for
// proc's session — the debounced broadcast callback autonomous spawns
// register with the cost store (mirrors broadcastLedgerEpoch's project-scope
// routing; console sessions register their own session-channel callback).
func (s *Spawner) broadcastSessionCost(proc *processInfo, snap CostSnapshot) {
	s.broadcast(ws.EventSessionCostUpdated, proc.projectID, proc.ticketID, proc.workflowName, map[string]interface{}{
		"session_id":    proc.sessionID,
		"cost_estimate": snap.CostUSD,
		"pricing_known": snap.PricingKnown,
	})
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

	// Register the session's running cost accounting for every mode that
	// tracks context (script agents never report usage, so registering for
	// them would sit permanently idle). Pricing resolves once here from
	// proc.modelID; addUsage/setUsage feed in from each engine's usage hook.
	if prep.executionMode != "script" {
		RegisterSessionCost(proc.sessionID, proc.modelID, s.pool(), s.config.Clock, func(snap CostSnapshot) {
			s.broadcastSessionCost(proc, snap)
		})
	}

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

	// Autonomous refinery sidecar: only cli_interactive spawns (the
	// long-running autonomous session shape) get one, and only when the
	// orchestrator wired a sidecar into this config (system one-offs never
	// do). See spawner/REFERENCE.md.
	if s.config.RefinerySidecar != nil && prep.executionMode == "cli_interactive" {
		s.config.RefinerySidecar.StartSession(proc.sessionID, proc.projectID, proc.workflowInstanceID, proc.nodeID)
	}
	return nil
}

// cancelRunningProcs kills every proc in running (SIGTERM, then SIGKILL after
// a 2s grace per process — a per-process select avoids a fixed sleep when
// processes exit quickly), marks each CANCELLED, flushes its messages, drops
// its context ledger, and registers the session stop. Called from
// monitorAll's ctx.Done() branch, which does not reach finalizePhase.
func (s *Spawner) cancelRunningProcs(ctx context.Context, running []*processInfo, req SpawnRequest) []*processInfo {
	logger.Warn(ctx, "agents cancelled", "count", len(running))
	for _, proc := range running {
		proc.backend.Kill(ctx, proc, syscall.SIGTERM)
	}
	completed := make([]*processInfo, 0, len(running))
	for _, proc := range running {
		select {
		case <-proc.doneCh:
		case <-time.After(2 * time.Second):
			proc.backend.Kill(ctx, proc, syscall.SIGKILL)
			<-proc.doneCh
		}
		proc.finalStatus = "CANCELLED"
		s.saveMessages(proc)
		s.registerAgentStopWithReason(req.ProjectID, req.TicketID, req.WorkflowName,
			proc.sessionID, proc.agentID, "fail", "cancelled", proc.modelID)
		globalLedgerStore.drop(proc.sessionID)
		if s.config.RefinerySidecar != nil {
			s.config.RefinerySidecar.StopSession(proc.sessionID)
		}
		FinalizeSessionCost(proc.sessionID)
		completed = append(completed, proc)
	}
	return completed
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
	if cfg, ok := s.config.ModelConfigs[model]; ok && cfg.CLIContext > 0 {
		return cfg.CLIContext
	}
	return 200000
}

// cliForModel derives the CLI from the registry provider. Unknown raw CLI
// model strings retain the historical Claude passthrough default.
func (s *Spawner) cliForModel(model string) string {
	if cfg, ok := s.config.ModelConfigs[model]; ok {
		return cliForProvider(cfg.Provider)
	}
	return "claude"
}

func cliForProvider(provider string) string {
	if provider == "openai" {
		return "codex"
	}
	return "claude"
}

func parseModelID(modelID string) (cli, model string) {
	if modelID == "" || !strings.Contains(modelID, ":") {
		return "claude", modelID
	}
	parts := strings.SplitN(modelID, ":", 2)
	return parts[0], parts[1]
}
