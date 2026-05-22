package spawner

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Codex per-session CODEX_HOME profile. No hooks are wired: codex never fires
// hooks under PTY (openai/codex#21639) and declaring any hook raises codex
// 0.133's blocking "N hooks need review" startup gate, which
// `--dangerously-bypass-hook-trust` does not clear. The profile's job is to
// keep the agent logged in (auth.json) and to grant workdir trust — codex 0.133
// reads the `[projects."<path>"] trust_level="trusted"` entry from
// CODEX_HOME/config.toml, and without it the TUI blocks on a directory-trust
// dialog even under `--dangerously-bypass-approvals-and-sandbox`.

// writeCodexProfileForSession writes CODEX_HOME/config.toml and copies the
// user's ~/.codex/auth.json (when present) so the agent stays logged in. The
// user's config.toml is preserved verbatim with `[[hooks.…]]` blocks stripped
// (they would otherwise trigger the hooks-review gate), and a
// `[projects."<resolvedWorkDir>"]` trust entry is appended. workDir is
// symlink-resolved to match codex's cwd canonicalization (e.g. `/var/folders`
// → `/private/var/folders` on macOS).
func writeCodexProfileForSession(dir, workDir string) error {
	userHome := userCodexHome()

	userTOML, _ := os.ReadFile(filepath.Join(userHome, "config.toml"))
	configTOML := string(stripHookTables(userTOML))
	if configTOML != "" && !strings.HasSuffix(configTOML, "\n") {
		configTOML += "\n"
	}
	if workDir != "" {
		resolved, err := filepath.EvalSymlinks(workDir)
		if err != nil {
			resolved = workDir
		}
		configTOML += fmt.Sprintf("\n[projects.%q]\ntrust_level = \"trusted\"\n", resolved)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(configTOML), 0o644); err != nil {
		return err
	}

	if authBytes, err := os.ReadFile(filepath.Join(userHome, "auth.json")); err == nil {
		_ = os.WriteFile(filepath.Join(dir, "auth.json"), authBytes, 0o600)
	}
	return nil
}

// stripHookTables removes every `[[hooks.<...>]]` (and `[hooks.<...>]`) block
// from the user's config so a user's own hook definitions don't trip codex
// 0.133's hooks-review gate in the spawned session. A block runs from its
// header through the line before the next top-level `[`/`[[…]]` header (or EOF).
func stripHookTables(toml []byte) []byte {
	lines := strings.Split(string(toml), "\n")
	out := make([]string, 0, len(lines))
	skipping := false
	for _, raw := range lines {
		trimmed := strings.TrimSpace(raw)
		if strings.HasPrefix(trimmed, "[") {
			skipping = strings.HasPrefix(trimmed, "[[hooks.") || strings.HasPrefix(trimmed, "[hooks.") || trimmed == "[hooks]"
		}
		if !skipping {
			out = append(out, raw)
		}
	}
	return []byte(strings.Join(out, "\n"))
}

func userCodexHome() string {
	if home := os.Getenv("HOME"); home != "" {
		return filepath.Join(home, ".codex")
	}
	return ".codex"
}
