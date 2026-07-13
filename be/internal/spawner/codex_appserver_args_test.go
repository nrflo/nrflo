package spawner

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestAppServerArgs_DisablesNativeMultiAgent verifies appServerArgs() starts
// with the "app-server" subcommand and denies each codexDisabledFeatures
// entry via a `--disable <feature>` pair, so that native multi-agent
// delegation cannot be reached even though multi_agent is stage=stable and
// default-ON in codex 0.144.1 (config.toml stripping alone would not disable
// it).
func TestAppServerArgs_DisablesNativeMultiAgent(t *testing.T) {
	t.Parallel()
	args := appServerArgs()

	if len(args) == 0 || args[0] != "app-server" {
		t.Fatalf("appServerArgs()[0] = %v, want \"app-server\": %v", args, args)
	}

	tests := []struct {
		feature string
	}{
		{"multi_agent"},
		{"multi_agent_v2"},
		{"enable_fanout"},
	}
	for _, tc := range tests {
		t.Run(tc.feature, func(t *testing.T) {
			pos := findArgElement(args, "--disable")
			found := false
			for i, a := range args {
				if a == "--disable" && i+1 < len(args) && args[i+1] == tc.feature {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("appServerArgs() missing --disable %s (first --disable at %d): %v", tc.feature, pos, args)
			}
		})
	}

	// Exact ordering/shape guard, matching codexDisabledFeatures declaration order.
	want := []string{"app-server", "--disable", "multi_agent", "--disable", "multi_agent_v2", "--disable", "enable_fanout"}
	if len(args) != len(want) {
		t.Fatalf("appServerArgs() = %v, want %v", args, want)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Errorf("appServerArgs()[%d] = %q, want %q: %v", i, args[i], want[i], args)
		}
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
