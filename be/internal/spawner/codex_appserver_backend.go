package spawner

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"syscall"
	"time"

	"be/internal/logger"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/ws"
)

// codexAppServerBackend drives `codex app-server` (newline-delimited JSON-RPC
// over stdio) for codex/cli_interactive agents, replacing the codex PTY/TUI.
// codex 0.133 emits no hooks under PTY (openai/codex#21639) and writes no
// rollout JSONL, so the PTY path can no longer surface agent messages or
// context_left. The app-server protocol exposes all of it as structured events
// (agentMessage, commandExecution, thread/tokenUsage/updated, turn lifecycle,
// typed rate-limit) which we map to the standard Sink.
//
// Completion stays socket/DB-driven: the agent runs `nrflo agent finished` via
// its shell tool (env inherited), the socket writes the result + dispatches a
// terminal signal → monitorAll → Kill → ctx cancel → run returns →
// handleCompletion reads the DB result (finalStatus left "" on the normal path).
// The PTY-only TUI bugs (terminal-query probes, input-box wrapping panic) do not
// apply here — there is no TUI.
type codexAppServerBackend struct {
	s          *Spawner
	mu         sync.Mutex
	cancel     context.CancelFunc
	client     *appServerClient
	profileDir string
}

func newCodexAppServerBackend(s *Spawner) *codexAppServerBackend {
	return &codexAppServerBackend{s: s}
}

func (b *codexAppServerBackend) Name() string                 { return "codex" }
func (b *codexAppServerBackend) SupportsResume() bool         { return false }
func (b *codexAppServerBackend) SupportsTakeControl() bool    { return false }
func (b *codexAppServerBackend) RequiresPrompt() bool         { return true }
func (b *codexAppServerBackend) TracksContext() bool          { return true }
func (b *codexAppServerBackend) ParsesStructuredOutput() bool { return false }

// NaturalExitGrace mirrors CodexAdapter — give the trailing turn/completed and
// final thread/tokenUsage/updated time to land before a forced kill.
func (b *codexAppServerBackend) NaturalExitGrace() time.Duration { return 2 * time.Second }

// Start builds the per-session CODEX_HOME profile, spawns `codex app-server`,
// and launches the run goroutine. Spawn-time failures (binary missing) return
// an error so the spawn fails loudly; handshake/runtime failures are handled
// inside the goroutine (proc.waitErr → handleCompletion).
func (b *codexAppServerBackend) Start(ctx context.Context, proc *processInfo, prep *prepResult) error {
	profileDir, err := os.MkdirTemp("", "nrflo-codex-as-"+proc.sessionID+"-*")
	if err != nil {
		return fmt.Errorf("codex app-server: mkdir profile: %w", err)
	}
	if err := writeCodexSessionProfile(profileDir, proc); err != nil {
		_ = os.RemoveAll(profileDir)
		return fmt.Errorf("codex app-server: write profile: %w", err)
	}
	b.profileDir = profileDir

	model := prep.opts.MappedModel
	if model == "" {
		model = (&CodexAdapter{}).MapModel(prep.opts.Model)
	}
	effort := prep.opts.ReasoningEffort
	if effort == "" {
		effort = (&CodexAdapter{}).GetReasoningEffort(prep.opts.Model)
	}

	env := removeEnvKey(prep.opts.Env, "CODEX_HOME=")
	env = append(env, "CODEX_HOME="+profileDir)
	if !envHasTERM(env) {
		env = append(env, "TERM=xterm-256color")
	}

	runCtx, cancel := context.WithCancel(ctx)
	client, err := startAppServer(runCtx, env, proc.workDir)
	if err != nil {
		cancel()
		_ = os.RemoveAll(profileDir)
		return fmt.Errorf("codex app-server: %w", err)
	}

	b.mu.Lock()
	b.cancel = cancel
	b.client = client
	b.mu.Unlock()

	proc.spawnCommand = fmt.Sprintf("codex app-server model=%s effort=%s", model, effort)
	proc.pid = 0 // client owns the exec.Cmd; Kill goes through ctx cancel.

	go b.run(runCtx, proc, prep, client, model, effort)
	return nil
}

// Kill cancels the run context (kills the app-server process) and closes the
// client. Signal is ignored — mirrors apiBackend.Kill.
func (b *codexAppServerBackend) Kill(ctx context.Context, proc *processInfo, sig syscall.Signal) error {
	b.mu.Lock()
	cancel, client := b.cancel, b.client
	b.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if client != nil {
		client.close()
	}
	return nil
}

const appServerClientName = "nrflo"

// run drives the JSON-RPC session: handshake → thread/start → turn/start →
// event loop. It always closes proc.doneCh and cleans up the profile dir.
func (b *codexAppServerBackend) run(runCtx context.Context, proc *processInfo, prep *prepResult, client *appServerClient, model, effort string) {
	defer close(proc.doneCh)
	defer client.close()
	defer os.RemoveAll(b.profileDir)

	sink := &spawnerSink{s: b.s}
	req := SpawnRequest{ProjectID: proc.projectID, TicketID: proc.ticketID, WorkflowName: proc.workflowName}
	logCtx := logger.WithTrx(context.Background(), proc.trx)

	if _, err := client.call(runCtx, "initialize", map[string]any{
		"clientInfo": map[string]string{"name": appServerClientName, "version": "1"},
	}); err != nil {
		b.fail(logCtx, proc, "initialize: "+err.Error())
		return
	}
	_ = client.notify("initialized", nil)

	resp, err := client.call(runCtx, "thread/start", map[string]any{
		"model":          model,
		"cwd":            proc.workDir,
		"sandbox":        "danger-full-access",
		"approvalPolicy": "never",
	})
	if err != nil {
		b.fail(logCtx, proc, "thread/start: "+err.Error())
		return
	}
	var threadResp struct {
		Thread struct {
			ID string `json:"id"`
		} `json:"thread"`
	}
	_ = json.Unmarshal(resp, &threadResp)
	threadID := threadResp.Thread.ID
	if threadID == "" {
		b.fail(logCtx, proc, "thread/start: empty thread id")
		return
	}

	if _, err := client.call(runCtx, "turn/start", turnStartParams(threadID, prep.prompt, effort, model)); err != nil {
		b.fail(logCtx, proc, "turn/start: "+err.Error())
		return
	}

	b.eventLoop(runCtx, logCtx, proc, req, client, threadID, effort, sink)
}

// eventLoop consumes notifications until the agent finishes (terminal signal →
// ctx cancel), the app-server crashes, a fatal error arrives, or a rate-limit
// triggers a restart. After a turn completes with no completion yet, an idle
// timer drives the nudge loop.
func (b *codexAppServerBackend) eventLoop(runCtx context.Context, logCtx context.Context, proc *processInfo, req SpawnRequest, client *appServerClient, threadID, effort string, sink Sink) {
	maxCtx := proc.maxContext
	turnActive := true // turn/start already issued

	for {
		idleCh := b.armIdleTimer(proc, turnActive)
		select {
		case <-runCtx.Done():
			// Terminal signal (agent finished) or cancellation. finalStatus
			// stays "" → monitorAll runs handleCompletion (reads DB result).
			return

		case <-client.closed:
			b.fail(logCtx, proc, "app-server connection closed")
			return

		case rpcReq := <-client.reqCh:
			// Defensive auto-approve (none expected under approvalPolicy:"never").
			if rpcReq.ID != nil {
				_ = client.reply(*rpcReq.ID, map[string]any{"decision": "approved"})
				logger.Info(logCtx, "codex app-server: auto-approved server request", "method", rpcReq.Method, "session_id", proc.sessionID)
			}

		case n := <-client.notifyCh:
			sig := dispatchAppServerEvent(proc.sessionID, n, sink, maxCtx)
			if sig.turnStarted {
				turnActive = true
			}
			if sig.turnCompleted {
				turnActive = false
			}
			if sig.rateLimited {
				b.handleRateLimit(proc, req, sig.matchedReason)
				return
			}
			if sig.fatalErr != "" {
				b.fail(logCtx, proc, "agent error: "+sig.fatalErr)
				return
			}

		case <-idleCh:
			if proc.nudgeCount < proc.nudgeMax {
				b.nudge(runCtx, logCtx, proc, req, client, threadID, effort)
				// The nudge issued a turn/start; a turn is now active. Mark it so
				// before turnStarted lands, so the next iteration arms no idle timer
				// (avoids a second nudge while the nudge turn is spinning up).
				turnActive = true
			} else if !proc.lastNudgeAt.IsZero() && b.s.config.Clock.Now().Sub(proc.lastNudgeAt) > b.betweenTurnsDelay(proc) {
				// Cap exhausted and the agent completed yet another turn without
				// finishing → auto-fail. This calls RequestTerminalSignal which
				// routes back through Kill → ctx cancel → handleCompletion reads
				// the fail result.
				b.s.handleNudgeAutoFail(logCtx, proc, req)
			}
		}
	}
}

// handleRateLimit mirrors apiBackend.Start's rate-limit dance: broadcast,
// register a continue stop, persist rate-limit state, wait, and set
// finalStatus=CONTINUE so monitorAll relaunches (or FAIL when exhausted).
func (b *codexAppServerBackend) handleRateLimit(proc *processInfo, req SpawnRequest, matched string) {
	b.s.saveMessages(proc)
	if !proc.rateLimitConfig.Enabled || proc.rateLimitTotalWait >= proc.rateLimitConfig.MaxWait {
		proc.finalStatus = "FAIL"
		b.s.registerAgentStopWithReason(proc.projectID, proc.ticketID, proc.workflowName,
			proc.sessionID, proc.agentID, "fail", "rate_limit_exhausted", proc.modelID)
		return
	}
	upcomingCount := proc.rateLimitRetryCount + 1
	delay := computeRateLimitDelay(proc.rateLimitConfig, upcomingCount)
	b.s.broadcast(ws.EventAgentRateLimited, proc.projectID, proc.ticketID, proc.workflowName, map[string]interface{}{
		"session_id":         proc.sessionID,
		"agent_type":         proc.agentType,
		"wait_seconds":       int(delay.Seconds()),
		"total_wait_seconds": int(proc.rateLimitTotalWait.Seconds()) + int(delay.Seconds()),
		"matched_pattern":    matched,
		"retry_count":        upcomingCount,
	})
	b.s.registerAgentStopWithReason(proc.projectID, proc.ticketID, proc.workflowName,
		proc.sessionID, proc.agentID, "continue", "rate_limit", proc.modelID)
	if pool := b.s.pool(); pool != nil {
		sessionRepo := repo.NewAgentSessionRepo(pool, b.s.config.Clock)
		sessionRepo.UpdateStatus(proc.sessionID, model.AgentSessionContinued)
		rateLimitUntil := b.s.config.Clock.Now().Add(delay).UTC().Format("2006-01-02T15:04:05.999999999Z07:00")
		sessionRepo.UpdateRateLimitUntil(proc.sessionID, rateLimitUntil, upcomingCount, "")
	}
	proc.rateLimitRetryCount++
	b.s.waitForRateLimitRetry(context.Background(), proc, req)
	proc.finalStatus = "CONTINUE"
}

// fail records a fatal error and marks the process exited-with-failure so
// handleCompletion classifies it (exit_code → ClassifyExit may reclassify as
// rate-limit). finalStatus stays "" so the DB result still wins if the agent
// happened to write one.
func (b *codexAppServerBackend) fail(logCtx context.Context, proc *processInfo, msg string) {
	logger.Warn(logCtx, "codex app-server: session failed", "session_id", proc.sessionID, "error", msg)
	proc.waitErr = fmt.Errorf("codex app-server: %s", msg)
	if b.s.config.ErrorSvc != nil {
		_ = b.s.config.ErrorSvc.RecordError(proc.projectID, "agent", proc.sessionID, "codex app-server: "+msg)
	}
}

// turnStartParams builds a turn/start params object. effort/model are omitted
// when empty.
func turnStartParams(threadID, text, effort, model string) map[string]any {
	p := map[string]any{
		"threadId": threadID,
		"input":    []map[string]any{{"type": "text", "text": text}},
	}
	if effort != "" {
		p["effort"] = effort
	}
	if model != "" {
		p["model"] = model
	}
	return p
}
