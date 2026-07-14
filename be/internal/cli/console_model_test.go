package cli

import (
	"context"
	"os/exec"
	"strings"
	"testing"

	"be/internal/model"
)

// captureRunConsoleChildArgs stubs runConsoleChild to record the built argv
// (never executing it — Rule 4) and return exitCode 0.
func captureRunConsoleChildArgs(t *testing.T) *[]string {
	t.Helper()
	var captured []string
	stubRunConsoleChild(t, func(cmd *exec.Cmd) (int, error) {
		captured = cmd.Args
		return 0, nil
	})
	return &captured
}

func modelFlagValue(t *testing.T, argv []string) (string, bool) {
	t.Helper()
	for i, a := range argv {
		if a == "--model" {
			if i+1 >= len(argv) {
				t.Fatalf("--model with no following value in argv %v", argv)
			}
			return argv[i+1], true
		}
	}
	return "", false
}

// TestRunConsole_ModelResolution_RegistryMappedModelWins covers case 3: the
// cli_models registry's mapped_model wins over the driver's own adapter
// MapModel fallback.
func TestRunConsole_ModelResolution_RegistryMappedModelWins(t *testing.T) {
	f := newFakeConsoleServer(t)
	f.cliModels = []model.CLIModel{
		{ID: "my_model", CLIType: "claude", MappedModel: "claude-registry-mapped", Enabled: true},
	}
	fakeBinDir(t, "claude")
	setConsoleFlags(t, "claude", "my_model", "p1", f.url(), f.serviceToken)
	argv := captureRunConsoleChildArgs(t)

	if _, err := runConsole(context.Background()); err != nil {
		t.Fatalf("runConsole() error: %v", err)
	}
	got, ok := modelFlagValue(t, *argv)
	if !ok || got != "claude-registry-mapped" {
		t.Errorf("--model = %q (present=%v), want claude-registry-mapped", got, ok)
	}
}

// TestRunConsole_ModelResolution_UnknownIDFallsBackToAdapterMapModel covers
// case 3: an id absent from the registry (found=false, no error) falls back
// to the driver's own adapter.MapModel — here ClaudeAdapter maps
// "opus_4_8" -> "claude-opus-4-8".
func TestRunConsole_ModelResolution_UnknownIDFallsBackToAdapterMapModel(t *testing.T) {
	f := newFakeConsoleServer(t) // empty cli-models registry
	fakeBinDir(t, "claude")
	setConsoleFlags(t, "claude", "opus_4_8", "p1", f.url(), f.serviceToken)
	argv := captureRunConsoleChildArgs(t)

	if _, err := runConsole(context.Background()); err != nil {
		t.Fatalf("runConsole() error: %v", err)
	}
	got, ok := modelFlagValue(t, *argv)
	if !ok || got != "claude-opus-4-8" {
		t.Errorf("--model = %q (present=%v), want claude-opus-4-8 (adapter fallback)", got, ok)
	}
}

// TestRunConsole_ModelResolution_EmptyModelOmitsFlag covers case 3: an empty
// --model skips the registry lookup entirely and omits --model from argv.
func TestRunConsole_ModelResolution_EmptyModelOmitsFlag(t *testing.T) {
	f := newFakeConsoleServer(t)
	fakeBinDir(t, "claude")
	setConsoleFlags(t, "claude", "", "p1", f.url(), f.serviceToken)
	argv := captureRunConsoleChildArgs(t)

	if _, err := runConsole(context.Background()); err != nil {
		t.Fatalf("runConsole() error: %v", err)
	}
	if _, ok := modelFlagValue(t, *argv); ok {
		t.Errorf("argv %v should not contain --model when --model is unset", *argv)
	}
}

// TestRunConsole_ModelResolution_CLITypeMismatchErrors covers case 3: a
// --model registered for the OTHER --cli (here opus_4_8, a claude id, passed
// with --cli codex) is a demonstrable user error, so it fails before launch
// rather than exec'ing the raw id and surfacing an opaque provider error
// inside the TUI with a console session already open.
func TestRunConsole_ModelResolution_CLITypeMismatchErrors(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // hermetic codex profile write
	f := newFakeConsoleServer(t)
	f.cliModels = []model.CLIModel{
		{ID: "opus_4_8", CLIType: "claude", MappedModel: "claude-opus-4-8", Enabled: true},
	}
	fakeBinDir(t, "codex")
	setConsoleFlags(t, "codex", "opus_4_8", "p1", f.url(), f.serviceToken)
	launched := false
	stubRunConsoleChild(t, func(*exec.Cmd) (int, error) { launched = true; return 0, nil })

	_, err := runConsole(context.Background())
	if err == nil {
		t.Fatal("runConsole() expected an error for a claude model id under --cli codex")
	}
	if !strings.Contains(err.Error(), "opus_4_8") || !strings.Contains(err.Error(), "claude") {
		t.Errorf("error %q should name the model and the cli_type it belongs to", err.Error())
	}
	if launched {
		t.Error("the CLI must not be launched when model resolution fails")
	}
}

// TestRunConsole_ModelResolution_DisabledRowErrors covers case 3: a disabled
// registry row is a user error too, not an unregistered raw model name.
func TestRunConsole_ModelResolution_DisabledRowErrors(t *testing.T) {
	f := newFakeConsoleServer(t)
	f.cliModels = []model.CLIModel{
		{ID: "retired", CLIType: "claude", MappedModel: "claude-retired", Enabled: false},
	}
	fakeBinDir(t, "claude")
	setConsoleFlags(t, "claude", "retired", "p1", f.url(), f.serviceToken)
	stubRunConsoleChild(t, func(*exec.Cmd) (int, error) { return 0, nil })

	_, err := runConsole(context.Background())
	if err == nil || !strings.Contains(err.Error(), "disabled") {
		t.Fatalf("runConsole() error = %v, want a 'disabled' error for a disabled registry row", err)
	}
}

// TestRunConsole_ModelResolution_RegistryEffortReachesArgv covers case 3: the
// registry row's reasoning_effort and fallback_models reach the claude argv.
// Without effort, ids that share a mapped_model (codex_gpt55_high vs _normal)
// would launch identically — effort is the only thing separating them.
func TestRunConsole_ModelResolution_RegistryEffortReachesArgv(t *testing.T) {
	f := newFakeConsoleServer(t)
	f.cliModels = []model.CLIModel{{
		ID: "opus_4_8", CLIType: "claude", MappedModel: "claude-opus-4-8",
		ReasoningEffort: "xhigh", FallbackModels: "sonnet", Enabled: true,
	}}
	fakeBinDir(t, "claude")
	setConsoleFlags(t, "claude", "opus_4_8", "p1", f.url(), f.serviceToken)
	argv := captureRunConsoleChildArgs(t)

	if _, err := runConsole(context.Background()); err != nil {
		t.Fatalf("runConsole() error: %v", err)
	}
	for _, want := range [][2]string{{"--effort", "xhigh"}, {"--fallback-model", "sonnet"}} {
		i := indexOfArg(*argv, want[0])
		if i == -1 || i+1 >= len(*argv) || (*argv)[i+1] != want[1] {
			t.Errorf("argv %v missing %s %s", *argv, want[0], want[1])
		}
	}
}

func indexOfArg(argv []string, want string) int {
	for i, a := range argv {
		if a == want {
			return i
		}
	}
	return -1
}
