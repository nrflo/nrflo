package spawner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestWriteCodexProfile_KeepsUserAgentsTable verifies a user's own [agents]
// table survives copying into the per-session config.toml verbatim —
// codexStripTablePrefixes deliberately has no "[agents" entry, since delegation
// is blocked via the `-c agents.enabled=false` argv override (appServerArgs),
// which deep-merges over this table rather than requiring it to be stripped.
func TestWriteCodexProfile_KeepsUserAgentsTable(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	codexHome := filepath.Join(fakeHome, ".codex")
	if err := os.MkdirAll(codexHome, 0o755); err != nil {
		t.Fatalf("mkdir codex home: %v", err)
	}
	userConfig := "model = \"gpt-5.4\"\n\n[agents]\nenabled = true\nmax_depth = 5\n"
	if err := os.WriteFile(filepath.Join(codexHome, "config.toml"), []byte(userConfig), 0o644); err != nil {
		t.Fatalf("write user config: %v", err)
	}

	dir := t.TempDir()
	if err := writeCodexProfileForSession(dir, ""); err != nil {
		t.Fatalf("writeCodexProfileForSession() error: %v", err)
	}
	content := readFileString(t, filepath.Join(dir, "config.toml"))
	if !strings.Contains(content, "[agents]\nenabled = true\nmax_depth = 5\n") {
		t.Errorf("config.toml does not preserve user [agents] table verbatim:\n%s", content)
	}
}
