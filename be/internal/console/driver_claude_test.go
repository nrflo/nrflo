package console

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// stubLookPath replaces the package's lookPath var for the duration of the
// test, restoring the original on cleanup — precedent:
// be/internal/service/cli_availability.go.
func stubLookPath(t *testing.T, fn func(string) (string, error)) {
	t.Helper()
	orig := lookPath
	lookPath = fn
	t.Cleanup(func() { lookPath = orig })
}

// TestClaudeDriver_Probe covers case 4: a missing binary surfaces an
// actionable error; a found binary probes clean.
func TestClaudeDriver_Probe(t *testing.T) {
	d := &claudeDriver{}

	stubLookPath(t, func(name string) (string, error) { return "", errors.New("not found") })
	if err := d.Probe(); err == nil {
		t.Fatal("Probe() expected an error when lookPath fails")
	}

	stubLookPath(t, func(name string) (string, error) { return "/usr/local/bin/" + name, nil })
	if err := d.Probe(); err != nil {
		t.Errorf("Probe() = %v, want nil when lookPath succeeds", err)
	}
}

// TestClaudeDriver_Prepare_ArgvExcludesManagedSessionFlags covers case 1: argv
// carries --mcp-config <path> and, with no model, nothing else — none of the
// spawner-managed-session flags (--dangerously-skip-permissions,
// --disallowedTools, --settings, --strict-mcp-config) ever appear for a human
// console session.
func TestClaudeDriver_Prepare_ArgvExcludesManagedSessionFlags(t *testing.T) {
	d := &claudeDriver{}
	spec, cleanup, err := d.Prepare(LaunchInput{NrfloPath: "/opt/nrflo_server", WorkDir: t.TempDir()})
	if err != nil {
		t.Fatalf("Prepare() error: %v", err)
	}
	t.Cleanup(cleanup)

	if len(spec.Argv) < 3 || spec.Argv[0] != "claude" || spec.Argv[1] != "--mcp-config" {
		t.Fatalf("Argv = %v, want [claude --mcp-config <path> ...]", spec.Argv)
	}
	cfgPath := spec.Argv[2]
	if _, err := os.Stat(cfgPath); err != nil {
		t.Errorf("mcp-config path %q does not exist: %v", cfgPath, err)
	}
	for _, banned := range []string{"--dangerously-skip-permissions", "--disallowedTools", "--settings", "--strict-mcp-config"} {
		for _, a := range spec.Argv {
			if a == banned {
				t.Errorf("Argv %v must not contain managed-session flag %q", spec.Argv, banned)
			}
		}
	}
}

// TestClaudeDriver_Prepare_ConfigFileContents covers case 1: the written
// mcp-config file wires the bridge as `nrflo -> agent mcp-external` with the
// bridge env, at file mode 0600 inside the temp dir.
func TestClaudeDriver_Prepare_ConfigFileContents(t *testing.T) {
	d := &claudeDriver{}
	in := LaunchInput{
		ServerURL:    "http://127.0.0.1:6587",
		ProjectID:    "proj-1",
		SessionID:    "sess-1",
		ConsoleToken: "console-bearer",
		WorkDir:      t.TempDir(),
		NrfloPath:    "/opt/nrflo_server",
	}
	spec, cleanup, err := d.Prepare(in)
	if err != nil {
		t.Fatalf("Prepare() error: %v", err)
	}
	t.Cleanup(cleanup)

	cfgPath := spec.Argv[2]
	info, err := os.Stat(cfgPath)
	if err != nil {
		t.Fatalf("stat config file: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("config file mode = %o, want 0600 (bearer must not be world/group readable)", perm)
	}

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("read config file: %v", err)
	}
	// consoleClaudeMCPConfig (spawner.WriteConsoleClaudeMCPConfig) is unexported
	// in the spawner package, so this test decodes into a local struct with the
	// same JSON shape rather than importing it.
	var cfg struct {
		MCPServers map[string]struct {
			Command string            `json:"command"`
			Args    []string          `json:"args"`
			Env     map[string]string `json:"env"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal config file: %v", err)
	}
	srv, ok := cfg.MCPServers["nrflo"]
	if !ok {
		t.Fatalf("config missing mcpServers.nrflo: %s", data)
	}
	if srv.Command != "/opt/nrflo_server" {
		t.Errorf("command = %q, want /opt/nrflo_server", srv.Command)
	}
	if len(srv.Args) != 2 || srv.Args[0] != "agent" || srv.Args[1] != "mcp-external" {
		t.Errorf("args = %v, want [agent mcp-external]", srv.Args)
	}
	for k, want := range map[string]string{
		"NRFLO_SERVER_URL":         "http://127.0.0.1:6587",
		"NRFLO_PROJECT":            "proj-1",
		"NRFLO_CONSOLE_TOKEN":      "console-bearer",
		"NRFLO_CONSOLE_SESSION_ID": "sess-1",
	} {
		if srv.Env[k] != want {
			t.Errorf("env[%q] = %q, want %q", k, srv.Env[k], want)
		}
	}
}

// TestClaudeDriver_Prepare_ModelResolution covers case 3: the registry mapped
// model wins verbatim, an empty MappedModel with a RawModel falls back to
// ClaudeAdapter.MapModel, and an empty model omits --model entirely.
func TestClaudeDriver_Prepare_ModelResolution(t *testing.T) {
	cases := []struct {
		name        string
		rawModel    string
		mappedModel string
		wantFlag    bool
		wantValue   string
	}{
		{name: "registry mapped model wins", rawModel: "opus_4_8", mappedModel: "claude-registry-override", wantFlag: true, wantValue: "claude-registry-override"},
		{name: "raw model falls back to adapter MapModel", rawModel: "opus_4_8", mappedModel: "", wantFlag: true, wantValue: "claude-opus-4-8"},
		{name: "unmapped raw model passes through adapter unchanged", rawModel: "custom-model-name", mappedModel: "", wantFlag: true, wantValue: "custom-model-name"},
		{name: "empty model omits the flag", rawModel: "", mappedModel: "", wantFlag: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := &claudeDriver{}
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

// TestClaudeDriver_Prepare_RegistryFlags covers case 3: the registry row's
// reasoning_effort and fallback_models become --effort/--fallback-model, the
// same flags the managed path passes (cli_adapter_claude.go:69-73), and are
// omitted when the row carries neither. ClaudeAdapter has no effort alias
// table, so there is no adapter fallback for a raw model id.
func TestClaudeDriver_Prepare_RegistryFlags(t *testing.T) {
	d := &claudeDriver{}
	spec, cleanup, err := d.Prepare(LaunchInput{
		NrfloPath:       "/opt/nrflo_server",
		WorkDir:         t.TempDir(),
		RawModel:        "opus_4_8",
		ReasoningEffort: "xhigh",
		FallbackModels:  "sonnet,haiku",
	})
	if err != nil {
		t.Fatalf("Prepare() error: %v", err)
	}
	t.Cleanup(cleanup)

	for _, want := range [][2]string{{"--effort", "xhigh"}, {"--fallback-model", "sonnet,haiku"}} {
		pos := indexOf(spec.Argv, want[0])
		if pos == -1 || pos+1 >= len(spec.Argv) {
			t.Fatalf("Argv %v missing %s <value>", spec.Argv, want[0])
		}
		if got := spec.Argv[pos+1]; got != want[1] {
			t.Errorf("%s value = %q, want %q", want[0], got, want[1])
		}
	}

	bare, cleanupBare, err := d.Prepare(LaunchInput{NrfloPath: "/opt/nrflo_server", WorkDir: t.TempDir(), RawModel: "opus_4_8"})
	if err != nil {
		t.Fatalf("Prepare() error: %v", err)
	}
	t.Cleanup(cleanupBare)
	for _, banned := range []string{"--effort", "--fallback-model"} {
		if indexOf(bare.Argv, banned) != -1 {
			t.Errorf("Argv %v should omit %s when the registry row has no value for it", bare.Argv, banned)
		}
	}
}

// TestClaudeDriver_Prepare_CleanupRemovesDir covers case 1: cleanup() removes
// the temp dir holding the mcp-config file.
func TestClaudeDriver_Prepare_CleanupRemovesDir(t *testing.T) {
	d := &claudeDriver{}
	spec, cleanup, err := d.Prepare(LaunchInput{NrfloPath: "/opt/nrflo_server", WorkDir: t.TempDir()})
	if err != nil {
		t.Fatalf("Prepare() error: %v", err)
	}
	dir := filepath.Dir(spec.Argv[2])
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("dir does not exist before cleanup: %v", err)
	}
	cleanup()
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("cleanup() did not remove dir %q (stat err: %v)", dir, err)
	}
	cleanup() // must not panic on a second call
}

func indexOf(s []string, v string) int {
	for i, x := range s {
		if x == v {
			return i
		}
	}
	return -1
}
