package spawner

import (
	"context"
	"fmt"
	"os"
	"strings"
	"syscall"
)

// cliInteractiveBackend runs a CLI agent inside a PTY without batch flags.
// The rendered prompt is delivered to the PTY via stdin Write after a short
// readiness delay. Take-control attaches a viewer without killing the agent.
//
// Backend selection: chosen when Config.InteractiveCLIMode is true and the
// adapter returns true for SupportsInteractive(). API-mode agents always use
// apiBackend regardless of the toggle.
type cliInteractiveBackend struct {
	adapter CLIAdapter
	s       *Spawner
	ptyMgr  ptyManagerIface
}

func newCLIInteractiveBackend(adapter CLIAdapter, s *Spawner, mgr ptyManagerIface) *cliInteractiveBackend {
	return &cliInteractiveBackend{adapter: adapter, s: s, ptyMgr: mgr}
}

func (b *cliInteractiveBackend) Name() string              { return "cli_interactive" }
func (b *cliInteractiveBackend) SupportsResume() bool      { return b.adapter.SupportsResume() }
func (b *cliInteractiveBackend) SupportsTakeControl() bool { return true }

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

	codexCleanup := func() {}
	var codexHome string
	if b.adapter.Name() == "codex" {
		dir, cleanup, err := BuildCodexHookProfile(proc)
		if err != nil {
			b.s.warnAgent(proc, "codex hook profile build failed: "+err.Error())
		} else {
			codexHome = dir
			codexCleanup = cleanup
		}
	}

	opts := InteractiveSpawnOptions{
		SessionID:        sessionID,
		Model:            model,
		ReasoningEffort:  prep.opts.ReasoningEffort,
		WorkDir:          workDir,
		Env:              env,
		SystemPromptFile: prep.suffixFile, // non-empty for Claude (written by prepareSpawn)
		SettingsJSON:     settingsJSON,
		CodexHome:        codexHome,
	}

	cmd := b.adapter.BuildInteractiveCommand(opts)

	// Build a forensics-friendly spawn command string.
	proc.spawnCommand = "interactive:" + strings.Join(cmd.Args, " ")

	// Register the command with the PTY manager then create the session.
	b.ptyMgr.RegisterCommand(sessionID, cmd.Path, cmd.Args[1:])
	sess, err := b.ptyMgr.Create(sessionID, workDir, env)
	if err != nil {
		if prep.suffixFile != "" {
			os.Remove(prep.suffixFile)
		}
		codexCleanup()
		return fmt.Errorf("cli_interactive: pty create: %w", err)
	}

	isClaude := b.adapter.Name() == "claude"

	// Deliver prompt body to PTY after readiness delay.
	go deliverPrompt(sess, prep.prompt)

	// Ferry PTY output to the spawner's message tracker.
	go ferryPTYOutput(b.s, proc, sess, isClaude)

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
		codexCleanup()
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

