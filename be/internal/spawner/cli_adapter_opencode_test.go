package spawner

import (
	"strconv"
	"strings"
	"testing"
)

// TestOpencodeAdapter_BuildInteractiveCommand_RequiredArgv verifies that the
// positional workdir, --port, --hostname 127.0.0.1, and --model appear in the
// built command's argument list.
func TestOpencodeAdapter_BuildInteractiveCommand_RequiredArgv(t *testing.T) {
	t.Parallel()
	a := &OpencodeAdapter{}
	opts := InteractiveSpawnOptions{
		Model:   "anthropic/claude-sonnet",
		WorkDir: "/projects/myrepo",
		Port:    54321,
	}
	cmd := a.BuildInteractiveCommand(opts)
	args := strings.Join(cmd.Args, " ")

	for _, want := range []string{
		"/projects/myrepo",
		"--port", "54321",
		"--hostname", "127.0.0.1",
		"--model", "anthropic/claude-sonnet",
	} {
		if !strings.Contains(args, want) {
			t.Errorf("BuildInteractiveCommand missing %q: %s", want, args)
		}
	}
}

// TestOpencodeAdapter_BuildInteractiveCommand_PortValueInArgv verifies the port
// number appears exactly once as a numeric token after --port.
func TestOpencodeAdapter_BuildInteractiveCommand_PortValueInArgv(t *testing.T) {
	t.Parallel()
	a := &OpencodeAdapter{}
	opts := InteractiveSpawnOptions{
		Model:   "anthropic/claude-sonnet",
		WorkDir: "/tmp",
		Port:    9999,
	}
	raw := a.BuildInteractiveCommand(opts).Args
	for i, arg := range raw {
		if arg == "--port" && i+1 < len(raw) {
			if raw[i+1] != strconv.Itoa(9999) {
				t.Errorf("--port value = %q, want %q", raw[i+1], "9999")
			}
			return
		}
	}
	t.Errorf("--port flag not found in argv: %v", raw)
}

// TestOpencodeAdapter_BuildInteractiveCommand_WithVariant verifies --variant
// is appended when ReasoningEffort is non-empty.
func TestOpencodeAdapter_BuildInteractiveCommand_WithVariant(t *testing.T) {
	t.Parallel()
	a := &OpencodeAdapter{}
	opts := InteractiveSpawnOptions{
		Model:           "openai/gpt-5.4",
		WorkDir:         "/tmp",
		Port:            1234,
		ReasoningEffort: "high",
	}
	args := strings.Join(a.BuildInteractiveCommand(opts).Args, " ")
	if !strings.Contains(args, "--variant high") {
		t.Errorf("BuildInteractiveCommand with ReasoningEffort=high missing --variant high: %s", args)
	}
}

// TestOpencodeAdapter_BuildInteractiveCommand_EmptyVariantOmitted verifies
// --variant is absent when ReasoningEffort is empty.
func TestOpencodeAdapter_BuildInteractiveCommand_EmptyVariantOmitted(t *testing.T) {
	t.Parallel()
	a := &OpencodeAdapter{}
	opts := InteractiveSpawnOptions{
		Model:   "anthropic/claude-sonnet",
		WorkDir: "/tmp",
		Port:    1234,
	}
	args := strings.Join(a.BuildInteractiveCommand(opts).Args, " ")
	if strings.Contains(args, "--variant") {
		t.Errorf("BuildInteractiveCommand with empty ReasoningEffort must not contain --variant: %s", args)
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

// TestOpencodeAdapter_PrepareInteractive_PortNonZero verifies that
// PrepareInteractive picks a free port (Port > 0).
func TestOpencodeAdapter_PrepareInteractive_PortNonZero(t *testing.T) {
	t.Parallel()
	a := &OpencodeAdapter{}
	extras, cleanup, err := a.PrepareInteractive(InteractivePrepOptions{
		SessionID: "sess-pi-1",
		WorkDir:   "/tmp",
	})
	if err != nil {
		t.Fatalf("PrepareInteractive: %v", err)
	}
	defer cleanup()
	if extras.Port == 0 {
		t.Error("PrepareInteractive returned Port=0, want a free port > 0")
	}
}

// TestOpencodeAdapter_PrepareInteractive_CleanupNonNil verifies that the
// returned cleanup func is non-nil and calling it does not panic.
func TestOpencodeAdapter_PrepareInteractive_CleanupNonNil(t *testing.T) {
	t.Parallel()
	a := &OpencodeAdapter{}
	_, cleanup, err := a.PrepareInteractive(InteractivePrepOptions{
		SessionID: "sess-pi-2",
		WorkDir:   "/tmp",
	})
	if err != nil {
		t.Fatalf("PrepareInteractive: %v", err)
	}
	if cleanup == nil {
		t.Fatal("PrepareInteractive returned nil cleanup func")
	}
	cleanup() // must not panic
}

// TestOpencodeAdapter_PrepareInteractive_PortInValidRange verifies that the
// picked port is within the valid TCP port range [1, 65535].
func TestOpencodeAdapter_PrepareInteractive_PortInValidRange(t *testing.T) {
	t.Parallel()
	a := &OpencodeAdapter{}
	extras, cleanup, err := a.PrepareInteractive(InteractivePrepOptions{
		SessionID: "sess-pi-3",
		WorkDir:   "/tmp",
	})
	if err != nil {
		t.Fatalf("PrepareInteractive: %v", err)
	}
	defer cleanup()
	if extras.Port < 1 || extras.Port > 65535 {
		t.Errorf("PrepareInteractive Port = %d, want in [1, 65535]", extras.Port)
	}
}

// TestOpencodeAdapter_VariantTableDriven covers the full BuildInteractiveCommand
// variant-injection table: present when effort is set, absent when empty.
func TestOpencodeAdapter_BuildInteractiveCommand_VariantTableDriven(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name            string
		reasoningEffort string
		wantVariant     bool
	}{
		{"high effort", "high", true},
		{"medium effort", "medium", true},
		{"low effort", "low", true},
		{"empty effort", "", false},
	}
	a := &OpencodeAdapter{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			opts := InteractiveSpawnOptions{
				Model:           "openai/gpt-5.4",
				WorkDir:         "/tmp",
				Port:            1234,
				ReasoningEffort: tc.reasoningEffort,
			}
			args := strings.Join(a.BuildInteractiveCommand(opts).Args, " ")
			hasVariant := strings.Contains(args, "--variant")
			if hasVariant != tc.wantVariant {
				t.Errorf("ReasoningEffort=%q: --variant present=%v, want %v; args: %s",
					tc.reasoningEffort, hasVariant, tc.wantVariant, args)
			}
			if tc.wantVariant && !strings.Contains(args, "--variant "+tc.reasoningEffort) {
				t.Errorf("ReasoningEffort=%q: --variant value wrong; args: %s",
					tc.reasoningEffort, args)
			}
		})
	}
}
