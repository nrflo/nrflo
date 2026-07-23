package spawner

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

// TestAppServerArgs_DisablesNativeDelegation verifies appServerArgs() starts
// with the "app-server" subcommand, contains no `--disable` element (the old
// multi_agent/multi_agent_v2/enable_fanout triple was a proven no-op and must
// never come back), and carries `-c agents.enabled=false` as a single argv
// pair blocking native delegation via the codex 0.145.0 `agents` config key.
func TestAppServerArgs_DisablesNativeDelegation(t *testing.T) {
	t.Parallel()
	args := appServerArgs()

	if len(args) == 0 || args[0] != "app-server" {
		t.Fatalf("appServerArgs()[0] = %v, want \"app-server\": %v", args, args)
	}

	// Regression guard: the dead --disable feature flags must never return.
	for i, a := range args {
		if a == "--disable" {
			t.Errorf("appServerArgs() contains --disable at %d (dead feature flag reintroduced): %v", i, args)
		}
	}

	// Exact ordering/shape guard.
	want := []string{
		"app-server",
		"-c", "agents.enabled=false",
		"-c", `project_doc_fallback_filenames=["AGENTS.md","CLAUDE.md"]`,
		"-c", "model_auto_compact_token_limit=1000000000",
	}
	if len(args) != len(want) {
		t.Fatalf("appServerArgs() = %v, want %v", args, want)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Errorf("appServerArgs()[%d] = %q, want %q: %v", i, args[i], want[i], args)
		}
	}
}

// TestAppServerArgs_LoadsClaudeMdFallback asserts the `-c
// project_doc_fallback_filenames=...` override is a single, unsplit argv
// element immediately following "-c" — a split/quoted value would be passed
// to codex as a literal string and silently ignored rather than parsed as a
// TOML array.
func TestAppServerArgs_LoadsClaudeMdFallback(t *testing.T) {
	t.Parallel()
	args := appServerArgs()

	const wantValue = `project_doc_fallback_filenames=["AGENTS.md","CLAUDE.md"]`
	found := false
	for i, a := range args {
		if a == "-c" && i+1 < len(args) && args[i+1] == wantValue {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("appServerArgs() missing single argv element \"-c\" %q: %v", wantValue, args)
	}
}

// TestCodexAgentsArgs_SingleUnsplitValue asserts codexAgentsArgs() returns
// "-c" followed immediately by ONE unsplit "agents.enabled=false" element —
// the same failure mode TestAppServerArgs_LoadsClaudeMdFallback guards for
// the project-doc key: a split/quoted value is passed to codex as a literal
// string and silently ignored rather than parsed as the intended -c override.
func TestCodexAgentsArgs_SingleUnsplitValue(t *testing.T) {
	t.Parallel()
	args := codexAgentsArgs()

	if len(args) != 2 || args[0] != "-c" || args[1] != "agents.enabled=false" {
		t.Fatalf("codexAgentsArgs() = %v, want [\"-c\" \"agents.enabled=false\"]", args)
	}
}

// TestCodexAutoCompactArgs_SingleUnsplitValue asserts codexAutoCompactArgs()
// returns "-c" followed immediately by ONE unsplit
// "model_auto_compact_token_limit=<N>" element — the same failure mode
// TestAppServerArgs_LoadsClaudeMdFallback guards for the project-doc key: a
// split/quoted value is passed to codex as a literal string and silently
// ignored rather than parsed as the intended `-c` override.
func TestCodexAutoCompactArgs_SingleUnsplitValue(t *testing.T) {
	t.Parallel()
	args := codexAutoCompactArgs()

	wantValue := fmt.Sprintf("model_auto_compact_token_limit=%d", codexAutoCompactTokenLimit)
	if len(args) != 2 || args[0] != "-c" || args[1] != wantValue {
		t.Fatalf("codexAutoCompactArgs() = %v, want [\"-c\" %q]", args, wantValue)
	}
}

// TestCodexConfigToml_NoProjectDocKey pins the design decision that
// project_doc_fallback_filenames is delivered exclusively via the argv -c
// override (appServerArgs), never written into the per-session config.toml.
// A future contributor "helpfully" adding it to config.toml would hard-fail
// every codex spawn with a duplicate-key parse error for any user who already
// sets this root-scope key in their own ~/.codex/config.toml.
func TestCodexConfigToml_NoProjectDocKey(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()

	if err := writeCodexProfileForSession(dir, ""); err != nil {
		t.Fatalf("writeCodexProfileForSession: %v", err)
	}

	content := readFileString(t, filepath.Join(dir, "config.toml"))
	if strings.Contains(content, "project_doc_fallback_filenames") {
		t.Errorf("config.toml must not contain project_doc_fallback_filenames (must be delivered exclusively via appServerArgs()'s -c override):\n%s", content)
	}
}

// TestCodexConfigToml_NoFeaturesTable guards against a future contributor
// "helpfully" adding a [features] table to the per-session config.toml: the
// app-server parses config.toml strictly, so a duplicate [features] table
// (this test's own --disable flags already cover feature-gating) would trip
// an rpc -32600. Native multi-agent delegation must stay blocked exclusively
// via appServerArgs()'s --disable flags, not config.toml.
func TestCodexConfigToml_NoFeaturesTable(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()

	if err := writeCodexProfileForSession(dir, ""); err != nil {
		t.Fatalf("writeCodexProfileForSession: %v", err)
	}

	content := readFileString(t, filepath.Join(dir, "config.toml"))
	if strings.Contains(content, "[features]") {
		t.Errorf("config.toml must not contain a [features] table (feature gating is exclusively via appServerArgs() --disable flags):\n%s", content)
	}
}
