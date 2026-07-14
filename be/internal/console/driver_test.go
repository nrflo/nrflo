package console

import (
	"strings"
	"testing"
)

// TestGetDriver_KnownNames covers case 7: claude and codex each resolve to a
// driver whose Name() matches the requested --cli value.
func TestGetDriver_KnownNames(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"claude", "codex"} {
		drv, err := GetDriver(name)
		if err != nil {
			t.Fatalf("GetDriver(%q): %v", name, err)
		}
		if drv.Name() != name {
			t.Errorf("GetDriver(%q).Name() = %q, want %q", name, drv.Name(), name)
		}
	}
}

// TestGetDriver_UnknownCLI covers case 7: an unknown --cli name errors instead
// of silently returning a nil driver.
func TestGetDriver_UnknownCLI(t *testing.T) {
	t.Parallel()
	drv, err := GetDriver("gemini")
	if err == nil {
		t.Fatal("GetDriver(\"gemini\") expected an error, got nil")
	}
	if drv != nil {
		t.Errorf("GetDriver(\"gemini\") expected a nil driver on error, got %#v", drv)
	}
	if !strings.Contains(err.Error(), "gemini") {
		t.Errorf("error %q should name the unknown CLI", err.Error())
	}
}

// TestWithCodexHome_AppendsNewKey verifies CODEX_HOME is appended when absent.
func TestWithCodexHome_AppendsNewKey(t *testing.T) {
	t.Parallel()
	got := withCodexHome([]string{"A=1", "B=2"}, "/tmp/x")
	want := []string{"A=1", "B=2", "CODEX_HOME=/tmp/x"}
	assertStringSlice(t, got, want)
}

// TestWithCodexHome_ReplacesExistingKey verifies an existing CODEX_HOME entry
// is removed before the new one is appended — a duplicate must never shadow
// ours (macOS getenv returns the first match in environ).
func TestWithCodexHome_ReplacesExistingKey(t *testing.T) {
	t.Parallel()
	got := withCodexHome([]string{"CODEX_HOME=/old", "A=1", "CODEX_HOME=/also-old"}, "/new")
	want := []string{"A=1", "CODEX_HOME=/new"}
	assertStringSlice(t, got, want)
	count := 0
	for _, e := range got {
		if strings.HasPrefix(e, "CODEX_HOME=") {
			count++
		}
	}
	if count != 1 {
		t.Errorf("CODEX_HOME appears %d times in %v, want exactly 1", count, got)
	}
}

// TestWithCodexHome_PrefixCollisionNotRemoved guards against a naive HasPrefix
// match: "CODEX_HOME_EXTRA=x" must survive.
func TestWithCodexHome_PrefixCollisionNotRemoved(t *testing.T) {
	t.Parallel()
	got := withCodexHome([]string{"CODEX_HOME_EXTRA=x"}, "/new")
	want := []string{"CODEX_HOME_EXTRA=x", "CODEX_HOME=/new"}
	assertStringSlice(t, got, want)
}

// TestBridgeEnv_CarriesConsoleIdentity verifies the required bridge vars are
// always present, and NRFLO_MCP_TOKEN is present only when a service token is
// supplied.
func TestBridgeEnv_CarriesConsoleIdentity(t *testing.T) {
	t.Parallel()
	in := LaunchInput{
		ServerURL:    "http://127.0.0.1:6587",
		ProjectID:    "proj-1",
		SessionID:    "sess-1",
		ConsoleToken: "console-bearer",
	}
	env := bridgeEnv(in)
	for k, want := range map[string]string{
		"NRFLO_SERVER_URL":         "http://127.0.0.1:6587",
		"NRFLO_PROJECT":            "proj-1",
		"NRFLO_CONSOLE_TOKEN":      "console-bearer",
		"NRFLO_CONSOLE_SESSION_ID": "sess-1",
	} {
		if env[k] != want {
			t.Errorf("bridgeEnv()[%q] = %q, want %q", k, env[k], want)
		}
	}
	if _, ok := env["NRFLO_MCP_TOKEN"]; ok {
		t.Errorf("NRFLO_MCP_TOKEN should be absent when ServiceToken is empty, got %q", env["NRFLO_MCP_TOKEN"])
	}

	in.ServiceToken = "svc-tok"
	env = bridgeEnv(in)
	if env["NRFLO_MCP_TOKEN"] != "svc-tok" {
		t.Errorf("NRFLO_MCP_TOKEN = %q, want svc-tok when ServiceToken is set", env["NRFLO_MCP_TOKEN"])
	}
}

func assertStringSlice(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("got[%d] = %q, want %q (full got=%v want=%v)", i, got[i], want[i], got, want)
		}
	}
}
