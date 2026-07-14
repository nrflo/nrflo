package spawner

import (
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
