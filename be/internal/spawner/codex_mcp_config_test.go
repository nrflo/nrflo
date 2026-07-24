package spawner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestAppendCodexMCPServer verifies the [mcp_servers.nrflo] table and its env
// table are appended to the per-session config.toml, with the bridge env
// embedded (codex does not forward parent env to MCP subprocesses).
func TestAppendCodexMCPServer(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	// Seed a base config.toml with a trust entry (append mode requires the file).
	if err := writeCodexProfileForSession(dir, ""); err != nil {
		t.Fatalf("writeCodexProfileForSession: %v", err)
	}

	env := nrfloBridgeEnv("sess-1", "wfi-1", "proj-1")
	if err := appendCodexMCPServer(dir, "/opt/nrflo_server", []string{"agent", "mcp"}, env); err != nil {
		t.Fatalf("appendCodexMCPServer: %v", err)
	}

	content := readFileString(t, filepath.Join(dir, "config.toml"))
	for _, want := range []string{
		"[mcp_servers.nrflo]",
		`command = "/opt/nrflo_server"`,
		`args = ["agent", "mcp"]`,
		"[mcp_servers.nrflo.env]",
		`NRF_SESSION_ID = "sess-1"`,
		`NRF_WORKFLOW_INSTANCE_ID = "wfi-1"`,
		`NRFLO_PROJECT = "proj-1"`,
	} {
		if !strings.Contains(content, want) {
			t.Errorf("config.toml missing %q\nfull:\n%s", want, content)
		}
	}
	// The env table must come after its parent server table.
	if strings.Index(content, "[mcp_servers.nrflo]") > strings.Index(content, "[mcp_servers.nrflo.env]") {
		t.Errorf("[mcp_servers.nrflo] must precede its .env table\nfull:\n%s", content)
	}
}

// TestWriteCodexProfile_StripsUserNrfloMCPServer guards the console-engine
// regression: a user who wired the nrflo mcp-external bridge into their own
// ~/.codex/config.toml would collide with the per-session [mcp_servers.nrflo]
// table appended here — the app-server's strict parser rejects the duplicate
// `nrflo` key (rpc -32600). The user's other MCP servers must survive.
func TestWriteCodexProfile_StripsUserNrfloMCPServer(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	codexHome := filepath.Join(fakeHome, ".codex")
	if err := os.MkdirAll(codexHome, 0o755); err != nil {
		t.Fatalf("mkdir codex home: %v", err)
	}
	userConfig := "model = \"gpt-5.4\"\n\n[mcp_servers.other]\ncommand = \"other-bin\"\n\n" +
		"[mcp_servers.nrflo]\ncommand = \"stale-bin\"\n\n[mcp_servers.nrflo.env]\nNRFLO_CONSOLE_TOKEN = \"stale\"\n\n" +
		"[mcp_servers.nrflo.tools.project_status]\nenabled = true\n"
	if err := os.WriteFile(filepath.Join(codexHome, "config.toml"), []byte(userConfig), 0o644); err != nil {
		t.Fatalf("write user config: %v", err)
	}

	dir := t.TempDir()
	if err := WriteConsoleCodexProfile(dir, "", "/opt/nrflo_server", nil); err != nil {
		t.Fatalf("WriteConsoleCodexProfile() error: %v", err)
	}
	content := readFileString(t, filepath.Join(dir, "config.toml"))
	if n := strings.Count(content, "[mcp_servers.nrflo]"); n != 1 {
		t.Errorf("[mcp_servers.nrflo] appears %d times, want exactly 1\nfull:\n%s", n, content)
	}
	for _, stale := range []string{"stale-bin", "NRFLO_CONSOLE_TOKEN = \"stale\"", "[mcp_servers.nrflo.tools."} {
		if strings.Contains(content, stale) {
			t.Errorf("stale user nrflo MCP content %q should be stripped\nfull:\n%s", stale, content)
		}
	}
	for _, keep := range []string{"[mcp_servers.other]", `command = "other-bin"`, `model = "gpt-5.4"`} {
		if !strings.Contains(content, keep) {
			t.Errorf("non-nrflo content %q should survive stripping\nfull:\n%s", keep, content)
		}
	}
}

// TestNrfloBridgeEnv_PropagatesSocket verifies socket-resolution vars are
// propagated when set on the server process.
func TestNrfloBridgeEnv_PropagatesSocket(t *testing.T) {
	t.Setenv("NRFLO_SOCKET", "/tmp/x.sock")
	t.Setenv("NRFLO_HOME", "/tmp/nrflohome")
	env := nrfloBridgeEnv("s", "w", "p")
	if env["NRFLO_SOCKET"] != "/tmp/x.sock" {
		t.Errorf("NRFLO_SOCKET = %q, want /tmp/x.sock", env["NRFLO_SOCKET"])
	}
	if env["NRFLO_HOME"] != "/tmp/nrflohome" {
		t.Errorf("NRFLO_HOME = %q, want /tmp/nrflohome", env["NRFLO_HOME"])
	}
	if env["NRF_SESSION_ID"] != "s" {
		t.Errorf("NRF_SESSION_ID = %q, want s", env["NRF_SESSION_ID"])
	}
}
