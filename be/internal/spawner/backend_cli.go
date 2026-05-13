package spawner

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"syscall"
	"time"
)

func newCLIBackend(adapter CLIAdapter, s *Spawner) *cliBackend {
	return &cliBackend{adapter: adapter, s: s}
}

type cliBackend struct {
	adapter CLIAdapter
	s       *Spawner
}

func (b *cliBackend) Name() string                 { return "cli" }
func (b *cliBackend) SupportsResume() bool         { return b.adapter.SupportsResume() }
func (b *cliBackend) SupportsTakeControl() bool    { return b.adapter.SupportsResume() }
func (b *cliBackend) RequiresPrompt() bool         { return true }
func (b *cliBackend) TracksContext() bool          { return true }
func (b *cliBackend) ParsesStructuredOutput() bool { return true }

// Start builds the exec.Cmd, wires stdin/stdout/stderr pipes, starts the process,
// and launches the output and wait goroutines. Sets proc.cmd and proc.spawnCommand.
func (b *cliBackend) Start(ctx context.Context, proc *processInfo, prep *prepResult) error {
	cmd := b.adapter.BuildCommand(prep.opts)

	removeSuffixFile := func() {
		if prep.suffixFile != "" {
			os.Remove(prep.suffixFile)
		}
	}

	var stdinFile *os.File
	if b.adapter.UsesStdinPrompt() {
		f, err := os.Open(prep.promptFile)
		if err != nil {
			os.Remove(prep.promptFile)
			removeSuffixFile()
			return fmt.Errorf("failed to open prompt file for stdin: %w", err)
		}
		cmd.Stdin = f
		stdinFile = f
	}

	// Capture spawn command for debugging/replay — prepend nrflo env vars
	// so the recorded command is fully reproducible.
	var envParts []string
	for _, e := range cmd.Env {
		if strings.HasPrefix(e, "NRF_") || strings.HasPrefix(e, "NRFLO_") {
			envParts = append(envParts, e)
		}
	}
	spawnCommand := strings.Join(cmd.Args, " ")
	if b.adapter.UsesStdinPrompt() {
		spawnCommand += " < " + prep.promptFile
	}
	if len(envParts) > 0 {
		spawnCommand = strings.Join(envParts, " ") + " " + spawnCommand
	}
	proc.spawnCommand = spawnCommand

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		if stdinFile != nil {
			stdinFile.Close()
		}
		os.Remove(prep.promptFile)
		removeSuffixFile()
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		if stdinFile != nil {
			stdinFile.Close()
		}
		os.Remove(prep.promptFile)
		removeSuffixFile()
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	spawnStartedAt := time.Now()
	if err := cmd.Start(); err != nil {
		if stdinFile != nil {
			stdinFile.Close()
		}
		os.Remove(prep.promptFile)
		removeSuffixFile()
		return fmt.Errorf("failed to start agent: %w", err)
	}
	if stdinFile != nil {
		stdinFile.Close()
	}

	proc.cmd = cmd

	// Optional post-spawn setup (e.g., codex rollout JSONL tailer, opencode SQLite tail).
	// Interface-asserted so adapters that don't need it (claude) are unaffected.
	postCleanup := func() {}
	if starter, ok := b.adapter.(PostStarter); ok {
		sink := &spawnerSink{s: b.s}
		cu, startErr := starter.PostStart(ctx, PostStartOptions{
			SessionID:  proc.sessionID,
			WorkDir:    prep.opts.WorkDir,
			Port:       0,
			CodexHome:  "",
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

	// Launch output monitoring goroutines.
	go b.s.monitorOutput(proc, stdout)
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			line := scanner.Text()
			b.s.warnAgent(proc, "[stderr] "+line)
			b.s.TrackMessage(proc, "[stderr] "+line, "text")
		}
	}()

	// Wait goroutine — closes doneCh when process exits.
	// Capture doneCh locally: proc.doneCh may be replaced during low-context save.
	origDoneCh := proc.doneCh
	promptPath := prep.promptFile
	suffixPath := prep.suffixFile
	go func() {
		proc.waitErr = cmd.Wait()
		close(origDoneCh)
		os.Remove(promptPath)
		if suffixPath != "" {
			os.Remove(suffixPath)
		}
		postCleanup()
	}()

	return nil
}

// Kill sends a signal to the running process. SIGKILL routes to Process.Kill();
// other signals route to Process.Signal. Nil-Process is silently ignored to
// preserve the previous safe-kill semantics at all call sites.
func (b *cliBackend) Kill(ctx context.Context, proc *processInfo, sig syscall.Signal) error {
	if proc.cmd == nil || proc.cmd.Process == nil {
		return nil
	}
	if sig == syscall.SIGKILL {
		return proc.cmd.Process.Kill()
	}
	return proc.cmd.Process.Signal(sig)
}
