package spawner

import (
	"strings"
	"testing"
)

// TestOpencodeAdapter_BuildInteractiveCommand_RequiredArgv verifies that the
// positional workdir, --port 0 (opencode self-allocates), --hostname
// 127.0.0.1, and --model appear in the built command's argument list.
func TestOpencodeAdapter_BuildInteractiveCommand_RequiredArgv(t *testing.T) {
	t.Parallel()
	a := &OpencodeAdapter{}
	opts := InteractiveSpawnOptions{
		Model:   "anthropic/claude-sonnet",
		WorkDir: "/projects/myrepo",
	}
	cmd := a.BuildInteractiveCommand(opts)
	args := strings.Join(cmd.Args, " ")

	for _, want := range []string{
		"/projects/myrepo",
		"--port", "0",
		"--hostname", "127.0.0.1",
		"--model", "anthropic/claude-sonnet",
	} {
		if !strings.Contains(args, want) {
			t.Errorf("BuildInteractiveCommand missing %q: %s", want, args)
		}
	}
}

// TestOpencodeAdapter_BuildInteractiveCommand_PortIsZero verifies that
// --port is always emitted as "0" so opencode itself picks a free port.
// Pre-allocating from nrflo's side caused a TOCTOU race under parallel
// spawning where the just-closed listener's port was grabbed by another
// process before opencode bound.
func TestOpencodeAdapter_BuildInteractiveCommand_PortIsZero(t *testing.T) {
	t.Parallel()
	a := &OpencodeAdapter{}
	opts := InteractiveSpawnOptions{
		Model:   "anthropic/claude-sonnet",
		WorkDir: "/tmp",
	}
	raw := a.BuildInteractiveCommand(opts).Args
	for i, arg := range raw {
		if arg == "--port" && i+1 < len(raw) {
			if raw[i+1] != "0" {
				t.Errorf("--port value = %q, want %q", raw[i+1], "0")
			}
			return
		}
	}
	t.Errorf("--port flag not found in argv: %v", raw)
}

// TestOpencodeAdapter_BuildInteractiveCommand_NoVariantFlag verifies that
// --variant is NEVER passed to the TUI subcommand, regardless of
// ReasoningEffort. The TUI subcommand (`opencode [project]`) doesn't
// accept --variant — only `opencode run` (batch) does. Passing it makes
// opencode print help to stderr and exit 1 before any prompt arrives.
func TestOpencodeAdapter_BuildInteractiveCommand_NoVariantFlag(t *testing.T) {
	t.Parallel()
	a := &OpencodeAdapter{}
	for _, effort := range []string{"", "low", "medium", "high"} {
		opts := InteractiveSpawnOptions{
			Model:           "openai/gpt-5.4",
			WorkDir:         "/tmp",
			ReasoningEffort: effort,
		}
		args := strings.Join(a.BuildInteractiveCommand(opts).Args, " ")
		if strings.Contains(args, "--variant") {
			t.Errorf("BuildInteractiveCommand(ReasoningEffort=%q) contained --variant; TUI subcommand rejects it: %s", effort, args)
		}
	}
}

// TestOpencodeAdapter_BuildInteractiveCommand_CmdDir verifies cmd.Dir is set to
// opts.WorkDir.
func TestOpencodeAdapter_BuildInteractiveCommand_CmdDir(t *testing.T) {
	t.Parallel()
	a := &OpencodeAdapter{}
	opts := InteractiveSpawnOptions{
		Model:   "anthropic/claude-sonnet",
		WorkDir: "/workspace/proj",
		Port:    5678,
	}
	cmd := a.BuildInteractiveCommand(opts)
	if cmd.Dir != "/workspace/proj" {
		t.Errorf("cmd.Dir = %q, want /workspace/proj", cmd.Dir)
	}
}

// TestOpencodeAdapter_BuildInteractiveCommand_TERMInjected verifies that
// TERM=xterm-256color is added to cmd.Env when the caller's env does not
// include any TERM assignment.
func TestOpencodeAdapter_BuildInteractiveCommand_TERMInjected(t *testing.T) {
	t.Parallel()
	a := &OpencodeAdapter{}
	opts := InteractiveSpawnOptions{
		Model:   "anthropic/claude-sonnet",
		WorkDir: "/tmp",
		Port:    1111,
		Env:     []string{"FOO=bar", "NRF_SESSION_ID=sess-1"},
	}
	cmd := a.BuildInteractiveCommand(opts)

	termFound := false
	for _, e := range cmd.Env {
		if strings.HasPrefix(e, "TERM=") {
			termFound = true
			if e != "TERM=xterm-256color" {
				t.Errorf("TERM env = %q, want TERM=xterm-256color", e)
			}
		}
	}
	if !termFound {
		t.Errorf("cmd.Env missing TERM=xterm-256color; env: %v", cmd.Env)
	}
}

// TestOpencodeAdapter_BuildInteractiveCommand_TERMPreserved verifies that when
// the caller already supplies a TERM assignment, the adapter does not add a
// second one (no duplicate TERM entries).
func TestOpencodeAdapter_BuildInteractiveCommand_TERMPreserved(t *testing.T) {
	t.Parallel()
	a := &OpencodeAdapter{}
	opts := InteractiveSpawnOptions{
		Model:   "anthropic/claude-sonnet",
		WorkDir: "/tmp",
		Port:    2222,
		Env:     []string{"TERM=xterm", "FOO=bar"},
	}
	cmd := a.BuildInteractiveCommand(opts)

	count := 0
	for _, e := range cmd.Env {
		if strings.HasPrefix(e, "TERM=") {
			count++
		}
	}
	if count != 1 {
		t.Errorf("cmd.Env has %d TERM entries, want exactly 1: %v", count, cmd.Env)
	}
}

// TestOpencodeAdapter_BuildInteractiveCommand_CallerEnvPreserved verifies that
// env vars supplied by the caller are present in cmd.Env unchanged.
func TestOpencodeAdapter_BuildInteractiveCommand_CallerEnvPreserved(t *testing.T) {
	t.Parallel()
	a := &OpencodeAdapter{}
	opts := InteractiveSpawnOptions{
		Model:   "anthropic/claude-sonnet",
		WorkDir: "/tmp",
		Port:    3333,
		Env:     []string{"NRF_SESSION_ID=sess-42", "NRFLO_PROJECT=proj-x"},
	}
	cmd := a.BuildInteractiveCommand(opts)

	envSet := make(map[string]bool, len(cmd.Env))
	for _, e := range cmd.Env {
		envSet[e] = true
	}
	for _, want := range []string{"NRF_SESSION_ID=sess-42", "NRFLO_PROJECT=proj-x"} {
		if !envSet[want] {
			t.Errorf("cmd.Env missing caller-supplied %q: %v", want, cmd.Env)
		}
	}
}

// TestOpencodeAdapter_PrepareInteractive_NoOp verifies that
// PrepareInteractive returns empty extras + non-nil cleanup. Port
// selection moved to opencode itself (`--port 0`) after the pre-allocate
// path was found to race under parallel spawning.
func TestOpencodeAdapter_PrepareInteractive_NoOp(t *testing.T) {
	t.Parallel()
	a := &OpencodeAdapter{}
	extras, cleanup, err := a.PrepareInteractive(InteractivePrepOptions{
		SessionID: "sess-pi-1",
		WorkDir:   "/tmp",
	})
	if err != nil {
		t.Fatalf("PrepareInteractive: %v", err)
	}
	if cleanup == nil {
		t.Fatal("PrepareInteractive returned nil cleanup func")
	}
	if extras.Port != 0 {
		t.Errorf("PrepareInteractive Port = %d, want 0 (opencode self-allocates)", extras.Port)
	}
	cleanup() // must not panic
}

// TestOpencodeAdapter_BuildInteractiveCommand_VariantNeverEmitted covers
// the full ReasoningEffort table and asserts --variant is omitted in
// every case. The TUI subcommand rejects the flag; only `opencode run`
// accepts it (see TestOpencodeAdapter_BuildCommand_* for batch).
func TestOpencodeAdapter_BuildInteractiveCommand_VariantNeverEmitted(t *testing.T) {
	t.Parallel()
	a := &OpencodeAdapter{}
	for _, effort := range []string{"high", "medium", "low", ""} {
		opts := InteractiveSpawnOptions{
			Model:           "openai/gpt-5.4",
			WorkDir:         "/tmp",
			ReasoningEffort: effort,
		}
		args := strings.Join(a.BuildInteractiveCommand(opts).Args, " ")
		if strings.Contains(args, "--variant") {
			t.Errorf("ReasoningEffort=%q: --variant must not appear (TUI rejects it): %s",
				effort, args)
		}
	}
}
