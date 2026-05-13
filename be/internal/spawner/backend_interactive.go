package spawner

import (
	"context"
	"fmt"
	"os"
	"strings"
	"syscall"
	"time"

	"be/internal/ws"
)

// cliInteractiveBackend runs a CLI agent inside a PTY without batch flags.
// The rendered prompt is delivered to the PTY via stdin Write after a short
// readiness delay. Take-control attaches a viewer without killing the agent.
//
// Backend selection: chosen when prep.executionMode == "cli_interactive".
// The execution_mode is set per-agent via agent_definitions.execution_mode or
// system_agent_definitions.execution_mode — no project-wide toggle is consulted.
type cliInteractiveBackend struct {
	adapter CLIAdapter
	s       *Spawner
	ptyMgr  ptyManagerIface
}

func newCLIInteractiveBackend(adapter CLIAdapter, s *Spawner, mgr ptyManagerIface) *cliInteractiveBackend {
	return &cliInteractiveBackend{adapter: adapter, s: s, ptyMgr: mgr}
}

func (b *cliInteractiveBackend) Name() string                  { return "cli_interactive" }
func (b *cliInteractiveBackend) SupportsResume() bool          { return b.adapter.SupportsResume() }
func (b *cliInteractiveBackend) SupportsTakeControl() bool     { return true }
func (b *cliInteractiveBackend) RequiresPrompt() bool          { return true }
func (b *cliInteractiveBackend) TracksContext() bool           { return true }
func (b *cliInteractiveBackend) ParsesStructuredOutput() bool  { return false }

// Start creates the PTY session, registers the command, delivers the rendered
// prompt body via stdin after a ~250ms readiness delay, and launches the ferry
// and wait goroutines. proc.cmd is left nil — PTY owns the process.
func (b *cliInteractiveBackend) Start(ctx context.Context, proc *processInfo, prep *prepResult) error {
	if b.ptyMgr == nil {
		return fmt.Errorf("cli_interactive backend: Config.PTYManager is nil")
	}

	sessionID := proc.sessionID
	workDir := prep.opts.WorkDir
	env := prep.opts.Env

	// Resolve mapped model (reuse DB-sourced value from opts, fall back to adapter).
	model := prep.opts.MappedModel
	if model == "" {
		model = b.adapter.MapModel(prep.opts.Model)
	}

	// Build settings JSON: merge safety hook JSON with interactive-mode hooks (T4 stub).
	settingsJSON := mergeInteractiveSettings(
		BuildInteractiveSettingsJSON(proc),
		prep.opts.SettingsJSON,
	)

	extras, prepCleanup, err := b.adapter.PrepareInteractive(InteractivePrepOptions{
		SessionID:          proc.sessionID,
		WorkflowInstanceID: proc.workflowInstanceID,
		ProjectID:          proc.projectID,
		WorkDir:            workDir,
	})
	if err != nil {
		b.s.warnAgent(proc, "interactive prep failed: "+err.Error())
	}
	if prepCleanup == nil {
		prepCleanup = func() {}
	}

	opts := InteractiveSpawnOptions{
		SessionID:        sessionID,
		Model:            model,
		ReasoningEffort:  prep.opts.ReasoningEffort,
		WorkDir:          workDir,
		Env:              env,
		SystemPromptFile: prep.suffixFile, // non-empty for Claude (written by prepareSpawn)
		SettingsJSON:     settingsJSON,
		CodexHome:        extras.CodexHome,
		Prompt:           prep.prompt, // Codex pre-loads via argv; other adapters ignore
		Hooks:            extras.Hooks,
		Port:             extras.Port, // embedded server port (opencode only; 0 for others)
	}

	cmd := b.adapter.BuildInteractiveCommand(opts)

	// Record the exact executable command (env-prefix + argv) for forensics.
	var envParts []string
	for _, e := range cmd.Env {
		if strings.HasPrefix(e, "NRF_") || strings.HasPrefix(e, "NRFLO_") {
			envParts = append(envParts, e)
		}
	}
	spawnCommand := strings.Join(cmd.Args, " ")
	if len(envParts) > 0 {
		spawnCommand = strings.Join(envParts, " ") + " " + spawnCommand
	}
	if prep.promptFile != "" {
		// Prompt body is written via PTY stdin Write after spawn, not via stdin
		// redirect — but the file path documents where the body lives for replay.
		spawnCommand += " < " + prep.promptFile
	}
	proc.spawnCommand = spawnCommand

	// Register the command with the PTY manager then create the session.
	// Capture wall-clock just before launch — opencode poller uses this to
	// disambiguate our session_id from prior history entries.
	spawnStartedAt := time.Now()
	b.ptyMgr.RegisterCommand(sessionID, cmd.Path, cmd.Args[1:])
	sess, err := b.ptyMgr.Create(sessionID, workDir, env)
	if err != nil {
		if prep.suffixFile != "" {
			os.Remove(prep.suffixFile)
		}
		prepCleanup()
		return fmt.Errorf("cli_interactive: pty create: %w", err)
	}
	proc.pid = sess.Pid()

	// Optional post-spawn setup (e.g., codex JSONL tailer, opencode SQLite tail).
	// Interface-asserted so adapters that don't need it are unaffected.
	postCleanup := func() {}
	if starter, ok := b.adapter.(PostStarter); ok {
		sink := &spawnerSink{s: b.s}
		cu, startErr := starter.PostStart(ctx, PostStartOptions{
			SessionID:  proc.sessionID,
			WorkDir:    workDir,
			Port:       extras.Port,
			CodexHome:  extras.CodexHome,
			StartedAt:  spawnStartedAt,
			MaxContext: b.s.maxContextForModel(prep.opts.Model),
			Sink:       sink,
		})
		if startErr != nil {
			b.s.warnAgent(proc, "PostStart failed: "+startErr.Error())
		} else if cu != nil {
			postCleanup = cu
		}
	}

	// Deliver prompt body to PTY after readiness delay. Adapters that deliver
	// the prompt themselves (codex, via argv positional) get an empty body so
	// deliverPrompt is a no-op for them.
	deliveryBody := prep.prompt
	if b.adapter.DeliversPromptInline() {
		deliveryBody = ""
	}
	go deliverPrompt(b.s, proc, sess, deliveryBody, b.adapter.Name(), proc.sessionStartCh, proc.firstByteCh)

	// Ferry PTY output (drop bytes). Auto-answer terminal capability queries
	// only for adapters that need them (codex).
	// BumpsOnPTYBytes gates lastMessageTime / hasReceivedMessage updates so
	// stall detection is reachable for hook/SSE/JSONL-driven adapters.
	go ferryPTYOutput(b.s, proc, sess, b.adapter.NeedsTerminalQueryReplies(), b.adapter.BumpsOnPTYBytes())

	// Wait goroutine: close doneCh when PTY session exits, clean up temp files.
	doneCh := proc.doneCh
	promptPath := prep.promptFile
	suffixPath := prep.suffixFile
	go func() {
		<-sess.Done()
		// Signal failure via waitErr if exit was non-zero — handleCompletion reads it.
		if code := exitCodeFromSession(sess); code != 0 {
			proc.waitErr = fmt.Errorf("pty process exited with code %d", code)
		}
		close(doneCh)
		os.Remove(promptPath)
		if suffixPath != "" {
			os.Remove(suffixPath)
		}
		postCleanup()
		prepCleanup()
	}()

	return nil
}

// Kill terminates the PTY session. Sends SIGTERM via Close(); if the session is
// still alive after graceTimeout escalates to SIGKILL.
func (b *cliInteractiveBackend) Kill(ctx context.Context, proc *processInfo, sig syscall.Signal) error {
	sess := b.ptyMgr.Get(proc.sessionID)
	if sess == nil {
		return nil
	}
	if sig == syscall.SIGKILL {
		return sess.Kill()
	}
	return sess.Close()
}

// exitCodeFromSession is a helper that reads the exit code from a ptySessionIface.
// Returns 0 when the concrete session type doesn't expose ExitCode.
func exitCodeFromSession(sess ptySessionIface) int {
	type exitCoder interface{ ExitCode() int }
	if ec, ok := sess.(exitCoder); ok {
		return ec.ExitCode()
	}
	return 0
}

// spawnerSink implements the Sink interface for the SSE event consumer,
// routing events through the spawner's existing service and broadcast paths.
type spawnerSink struct {
	s *Spawner
}

func (ss *spawnerSink) RecordHookMessage(sessionID, content, category, payload string) (string, string, string, error) {
	if ss.s.config.AgentSvcReal == nil {
		return "", "", "", nil
	}
	return ss.s.config.AgentSvcReal.RecordHookMessage(sessionID, content, category, payload)
}

func (ss *spawnerSink) UpdateContextLeft(sessionID string, pct int) (string, string, string, error) {
	if ss.s.config.AgentSvcReal == nil {
		return "", "", "", nil
	}
	projectID, ticketID, workflowName, err := ss.s.config.AgentSvcReal.UpdateContextLeft(sessionID, pct)
	if err == nil && projectID != "" {
		ss.s.broadcast(ws.EventAgentContextUpdated, projectID, ticketID, workflowName, map[string]interface{}{
			"session_id":   sessionID,
			"context_left": pct,
		})
	}
	return projectID, ticketID, workflowName, err
}

func (ss *spawnerSink) BumpLastMessage(sessionID string) {
	ss.s.BumpLastMessage(sessionID)
}

func (ss *spawnerSink) SetLastMessage(sessionID, content string) {
	ss.s.SetLastMessage(sessionID, content)
}

func (ss *spawnerSink) OnTurnComplete(sessionID string) {
	// Reset idle window by bumping the last-message timestamp.
	ss.s.BumpLastMessage(sessionID)
}

func (ss *spawnerSink) BroadcastMessagesUpdated(projectID, ticketID, workflow, sessionID string) {
	ss.s.broadcast(ws.EventMessagesUpdated, projectID, ticketID, workflow, map[string]interface{}{
		"session_id": sessionID,
	})
}

func (ss *spawnerSink) RecordError(projectID, errType, sessionID, msg string) {
	if ss.s.config.ErrorSvc != nil {
		ss.s.config.ErrorSvc.RecordError(projectID, errType, sessionID, msg)
	}
}
