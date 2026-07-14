package cli

import (
	"errors"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"be/internal/console"
)

// buildConsoleCmd turns a console.LaunchSpec into an *exec.Cmd with the real
// terminal inherited — no PTY. The child owns the foreground: it reads/writes
// the terminal directly, exactly as if the user had typed `claude`/`codex`
// themselves.
func buildConsoleCmd(spec console.LaunchSpec) *exec.Cmd {
	cmd := exec.Command(spec.Argv[0], spec.Argv[1:]...)
	cmd.Env = spec.Env
	cmd.Dir = spec.Dir
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd
}

// runConsoleChild starts cmd, waits for it, and returns its exit code. A
// package var so tests stub it out entirely — Rule 4 forbids real CLI
// execution in tests.
//
// SIGINT is swallowed here: the child is in the terminal's foreground process
// group and receives SIGINT directly from the tty, so the parent doing
// nothing is correct — it must stay alive to run the deferred session close.
// SIGTERM (no controlling-tty equivalent) is forwarded to the child so an
// external `kill` still lets it shut down before the parent closes the
// session.
var runConsoleChild = func(cmd *exec.Cmd) (int, error) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	if err := cmd.Start(); err != nil {
		return -1, err
	}

	done := make(chan struct{})
	go func() {
		for {
			select {
			case sig := <-sigCh:
				if sig == syscall.SIGTERM && cmd.Process != nil {
					_ = cmd.Process.Signal(syscall.SIGTERM)
				}
			case <-done:
				return
			}
		}
	}()

	waitErr := cmd.Wait()
	close(done)

	if waitErr == nil {
		return 0, nil
	}
	var exitErr *exec.ExitError
	if errors.As(waitErr, &exitErr) {
		// ExitCode() is -1 for a signal-terminated child, and os.Exit(-1) is
		// truncated by the kernel to 255. Report the shell convention instead:
		// 128+signal (130 for SIGINT, 143 for SIGTERM).
		if ws, ok := exitErr.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
			return 128 + int(ws.Signal()), nil
		}
		return exitErr.ExitCode(), nil
	}
	return -1, waitErr
}
