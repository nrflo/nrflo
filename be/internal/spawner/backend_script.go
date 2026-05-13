package spawner

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

// scriptBackend executes a stored Python script as an agent. The script code is
// written to /tmp/nrflo/scripts/<sessionID>.py and run via python3. Stdout is
// tracked as plain text (ParsesStructuredOutput=false). The temp file is removed
// after the process exits.
type scriptBackend struct {
	s *Spawner
}

func newScriptBackend(s *Spawner) *scriptBackend {
	return &scriptBackend{s: s}
}

func (b *scriptBackend) Name() string                  { return "script" }
func (b *scriptBackend) SupportsResume() bool          { return false }
func (b *scriptBackend) SupportsTakeControl() bool     { return false }
func (b *scriptBackend) RequiresPrompt() bool          { return false }
func (b *scriptBackend) TracksContext() bool           { return false }
func (b *scriptBackend) ParsesStructuredOutput() bool   { return false }
// NaturalExitGrace returns 0 — script backend is a one-shot python3
// invocation that exits as soon as its main returns; no end-of-turn
// flush to wait for.
func (b *scriptBackend) NaturalExitGrace() time.Duration { return 0 }

// Start writes the script to a temp file and spawns python3. Stdout is piped
// through monitorOutput (which routes to TrackMessage because
// ParsesStructuredOutput=false). Stderr is logged per line. The wait goroutine
// closes proc.doneCh and removes the script file on exit.
func (b *scriptBackend) Start(ctx context.Context, proc *processInfo, prep *prepResult) error {
	scriptDir := "/tmp/nrflo/scripts"
	if err := os.MkdirAll(scriptDir, 0o755); err != nil {
		return fmt.Errorf("script backend: mkdir: %w", err)
	}
	scriptPath := filepath.Join(scriptDir, proc.sessionID+".py")
	if err := os.WriteFile(scriptPath, []byte(prep.scriptCode), 0o600); err != nil {
		return fmt.Errorf("script backend: write script: %w", err)
	}

	env := prep.opts.Env
	if b.s.config.SDKDir != "" {
		env = append(env, "NRFLO_SDK_DIR="+b.s.config.SDKDir)
	}
	pyBin := resolvePythonBin(prep)
	cmd := exec.CommandContext(ctx, pyBin, scriptPath)
	cmd.Dir = prep.opts.WorkDir
	cmd.Env = env

	proc.spawnCommand = fmt.Sprintf("%s %s", pyBin, scriptPath)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		os.Remove(scriptPath)
		return fmt.Errorf("script backend: stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		os.Remove(scriptPath)
		return fmt.Errorf("script backend: stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		os.Remove(scriptPath)
		return fmt.Errorf("script backend: start: %w", err)
	}
	proc.cmd = cmd

	go b.s.monitorOutput(proc, stdout)
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			line := scanner.Text()
			b.s.warnAgent(proc, "[stderr] "+line)
			b.s.TrackMessage(proc, "[stderr] "+line, "text")
		}
	}()

	origDoneCh := proc.doneCh
	go func() {
		proc.waitErr = cmd.Wait()
		close(origDoneCh)
		os.Remove(scriptPath)
	}()

	return nil
}

// resolvePythonBin returns the python binary to use for a script-mode agent.
// When prep.pythonPath is non-empty (venv resolved by orchestrator), it is used.
// Otherwise falls back to "python3" on PATH.
func resolvePythonBin(prep *prepResult) string {
	if prep.pythonPath != "" {
		return prep.pythonPath
	}
	return "python3"
}

// Kill sends a signal to the running python3 process. SIGKILL routes to
// Process.Kill(); other signals route to Process.Signal. Nil-safe.
func (b *scriptBackend) Kill(ctx context.Context, proc *processInfo, sig syscall.Signal) error {
	if proc.cmd == nil || proc.cmd.Process == nil {
		return nil
	}
	if sig == syscall.SIGKILL {
		return proc.cmd.Process.Kill()
	}
	return proc.cmd.Process.Signal(sig)
}
