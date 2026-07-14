package console

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCodexDriver_Probe mirrors TestClaudeDriver_Probe for the codex driver.
func TestCodexDriver_Probe(t *testing.T) {
	d := &codexDriver{}

	stubLookPath(t, func(name string) (string, error) { return "", errors.New("not found") })
	if err := d.Probe(); err == nil {
		t.Fatal("Probe() expected an error when lookPath fails")
	}

	stubLookPath(t, func(name string) (string, error) { return "/usr/local/bin/" + name, nil })
	if err := d.Probe(); err != nil {
		t.Errorf("Probe() = %v, want nil when lookPath succeeds", err)
	}
}

// TestCodexDriver_Prepare_ArgvExcludesBypassFlag covers case 2: the codex
// console argv never carries --dangerously-bypass-approvals-and-sandbox — a
// human at a real terminal keeps codex's own approval/sandbox prompts.
func TestCodexDriver_Prepare_ArgvExcludesBypassFlag(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	d := &codexDriver{}
	spec, cleanup, err := d.Prepare(LaunchInput{NrfloPath: "/opt/nrflo_server", WorkDir: t.TempDir()})
	if err != nil {
		t.Fatalf("Prepare() error: %v", err)
	}
	t.Cleanup(cleanup)

	if len(spec.Argv) == 0 || spec.Argv[0] != "codex" {
		t.Fatalf("Argv = %v, want to start with codex", spec.Argv)
	}
	for _, a := range spec.Argv {
		if a == "--dangerously-bypass-approvals-and-sandbox" {
			t.Errorf("Argv %v must not contain --dangerously-bypass-approvals-and-sandbox", spec.Argv)
		}
	}
}

// TestCodexDriver_Prepare_CodexHomeEnv covers case 2: CODEX_HOME appears
// exactly once in Env and points at the temp profile dir, even when the
// parent process already had CODEX_HOME set to something else.
func TestCodexDriver_Prepare_CodexHomeEnv(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CODEX_HOME", "/inherited/stale/codex/home")
	d := &codexDriver{}
	spec, cleanup, err := d.Prepare(LaunchInput{NrfloPath: "/opt/nrflo_server", WorkDir: t.TempDir()})
	if err != nil {
		t.Fatalf("Prepare() error: %v", err)
	}
	t.Cleanup(cleanup)

	var matches []string
	for _, e := range spec.Env {
		if strings.HasPrefix(e, "CODEX_HOME=") {
			matches = append(matches, e)
		}
	}
	if len(matches) != 1 {
		t.Fatalf("CODEX_HOME entries = %v, want exactly 1 (inherited value must be stripped)", matches)
	}
	if matches[0] == "CODEX_HOME=/inherited/stale/codex/home" {
		t.Errorf("CODEX_HOME still carries the inherited parent value: %q", matches[0])
	}
}

// TestCodexDriver_Prepare_ConfigTOML covers case 2: the profile config.toml
// carries the [mcp_servers.nrflo] table wired to `agent mcp-external`, its env
// table, and the workdir trust entry.
func TestCodexDriver_Prepare_ConfigTOML(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	d := &codexDriver{}
	workDir := t.TempDir()
	in := LaunchInput{
		ServerURL:    "http://127.0.0.1:6587",
		ProjectID:    "proj-1",
		SessionID:    "sess-1",
		ConsoleToken: "console-bearer",
		WorkDir:      workDir,
		NrfloPath:    "/opt/nrflo_server",
	}
	spec, cleanup, err := d.Prepare(in)
	if err != nil {
		t.Fatalf("Prepare() error: %v", err)
	}
	t.Cleanup(cleanup)

	var codexHome string
	for _, e := range spec.Env {
		if strings.HasPrefix(e, "CODEX_HOME=") {
			codexHome = strings.TrimPrefix(e, "CODEX_HOME=")
		}
	}
	if codexHome == "" {
		t.Fatal("CODEX_HOME not found in Env")
	}
	data, err := os.ReadFile(filepath.Join(codexHome, "config.toml"))
	if err != nil {
		t.Fatalf("read config.toml: %v", err)
	}
	content := string(data)
	for _, want := range []string{
		"[mcp_servers.nrflo]",
		`command = "/opt/nrflo_server"`,
		`args = ["agent", "mcp-external"]`,
		"[mcp_servers.nrflo.env]",
		`NRFLO_SERVER_URL = "http://127.0.0.1:6587"`,
		`NRFLO_PROJECT = "proj-1"`,
		`NRFLO_CONSOLE_TOKEN = "console-bearer"`,
		`NRFLO_CONSOLE_SESSION_ID = "sess-1"`,
	} {
		if !strings.Contains(content, want) {
			t.Errorf("config.toml missing %q\nfull:\n%s", want, content)
		}
	}
	resolved, _ := filepath.EvalSymlinks(workDir)
	if !strings.Contains(content, "trust_level = \"trusted\"") || !strings.Contains(content, resolved) {
		t.Errorf("config.toml missing workdir trust entry for %q\nfull:\n%s", resolved, content)
	}
}

// TestCodexDriver_Prepare_ModelResolution mirrors the claude-driver table:
// registry mapped model wins, empty MappedModel falls back to
// CodexAdapter.MapModel, empty model omits --model.
func TestCodexDriver_Prepare_ModelResolution(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cases := []struct {
		name        string
		rawModel    string
		mappedModel string
		wantFlag    bool
		wantValue   string
	}{
		{name: "registry mapped model wins", rawModel: "codex_gpt55_high", mappedModel: "gpt-registry-override", wantFlag: true, wantValue: "gpt-registry-override"},
		{name: "raw model falls back to adapter MapModel", rawModel: "codex_gpt55_high", mappedModel: "", wantFlag: true, wantValue: "gpt-5.5"},
		{name: "empty model omits the flag", rawModel: "", mappedModel: "", wantFlag: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := &codexDriver{}
			spec, cleanup, err := d.Prepare(LaunchInput{
				NrfloPath:   "/opt/nrflo_server",
				WorkDir:     t.TempDir(),
				RawModel:    tc.rawModel,
				MappedModel: tc.mappedModel,
			})
			if err != nil {
				t.Fatalf("Prepare() error: %v", err)
			}
			t.Cleanup(cleanup)

			pos := indexOf(spec.Argv, "--model")
			if tc.wantFlag {
				if pos == -1 || pos+1 >= len(spec.Argv) {
					t.Fatalf("Argv %v missing --model <value>", spec.Argv)
				}
				if got := spec.Argv[pos+1]; got != tc.wantValue {
					t.Errorf("--model value = %q, want %q", got, tc.wantValue)
				}
			} else if pos != -1 {
				t.Errorf("Argv %v should not contain --model", spec.Argv)
			}
		})
	}
}

// TestCodexDriver_Prepare_ReasoningEffort covers case 2: effort reaches codex
// as a `-c model_reasoning_effort=...` override (the TUI has no --effort flag),
// registry row first, else CodexAdapter's own alias table. This is what keeps
// codex_gpt55_high distinct from codex_gpt55_normal — both MapModel to gpt-5.5,
// so without effort the two launches would be byte-identical.
func TestCodexDriver_Prepare_ReasoningEffort(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cases := []struct {
		name       string
		rawModel   string
		regEffort  string
		wantEffort string
	}{
		{name: "registry effort wins", rawModel: "codex_gpt55_high", regEffort: "low", wantEffort: "low"},
		{name: "high alias falls back to adapter effort", rawModel: "codex_gpt55_high", wantEffort: "high"},
		{name: "normal alias falls back to adapter effort", rawModel: "codex_gpt55_normal", wantEffort: "medium"},
		{name: "no model, no effort", rawModel: "", wantEffort: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := &codexDriver{}
			spec, cleanup, err := d.Prepare(LaunchInput{
				NrfloPath:       "/opt/nrflo_server",
				WorkDir:         t.TempDir(),
				RawModel:        tc.rawModel,
				ReasoningEffort: tc.regEffort,
			})
			if err != nil {
				t.Fatalf("Prepare() error: %v", err)
			}
			t.Cleanup(cleanup)

			pos := indexOf(spec.Argv, "-c")
			if tc.wantEffort == "" {
				if pos != -1 {
					t.Errorf("Argv %v should carry no -c override when there is no model", spec.Argv)
				}
				return
			}
			if pos == -1 || pos+1 >= len(spec.Argv) {
				t.Fatalf("Argv %v missing -c model_reasoning_effort override", spec.Argv)
			}
			want := `model_reasoning_effort="` + tc.wantEffort + `"`
			if got := spec.Argv[pos+1]; got != want {
				t.Errorf("-c value = %q, want %q", got, want)
			}
		})
	}
}

// TestCodexDriver_Prepare_CleanupRemovesDir covers case 2: cleanup() removes
// the CODEX_HOME profile dir.
func TestCodexDriver_Prepare_CleanupRemovesDir(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	d := &codexDriver{}
	spec, cleanup, err := d.Prepare(LaunchInput{NrfloPath: "/opt/nrflo_server", WorkDir: t.TempDir()})
	if err != nil {
		t.Fatalf("Prepare() error: %v", err)
	}
	var codexHome string
	for _, e := range spec.Env {
		if strings.HasPrefix(e, "CODEX_HOME=") {
			codexHome = strings.TrimPrefix(e, "CODEX_HOME=")
		}
	}
	if _, err := os.Stat(codexHome); err != nil {
		t.Fatalf("dir does not exist before cleanup: %v", err)
	}
	cleanup()
	if _, err := os.Stat(codexHome); !os.IsNotExist(err) {
		t.Errorf("cleanup() did not remove dir %q (stat err: %v)", codexHome, err)
	}
	cleanup() // must not panic on a second call
}
