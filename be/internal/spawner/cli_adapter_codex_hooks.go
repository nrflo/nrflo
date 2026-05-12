package spawner

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// codex hook profile + hook command helpers — package-internal, called only
// from cli_adapter_codex.go (PrepareInteractive). Codex's TUI in PTY contexts
// ignores `<CODEX_HOME>/config.toml` hook tables entirely, so this profile
// only carries auth, model/personality, the workDir trust entry, and the
// `[features] codex_hooks = true` flag (documented in https://developers.openai.com/codex/hooks).
// NOTE: codex 0.129.0-alpha.15+ has a regression where hooks do not fire
// regardless of declaration mechanism (upstream issue openai/codex#21639,
// open as of 2026-05). Tracked in backlog.md. Hooks themselves are injected via
// repeated `-c hooks.<event>=…` flags from BuildInteractiveCommand.

// writeCodexProfile is a convenience wrapper for tests that don't need
// per-session env injection.
func writeCodexProfile(dir, nrfloPath string) error {
	return writeCodexProfileForSession(dir, nrfloPath, "", "", "", "")
}

// writeCodexProfileForSession writes config.toml and copies the user's
// ~/.codex/auth.json (when present) so the agent stays logged in. The user's
// existing config.toml is preserved verbatim with `[[hooks.…]]` blocks
// stripped (those would compete with our `-c`-injected hooks), `[features]
// codex_hooks = true` ensured, and a `[projects."<workDir>"]` trust entry
// appended so codex doesn't block on its trust dialog.
func writeCodexProfileForSession(dir, nrfloPath, sessionID, instanceID, projectID, workDir string) error {
	_ = sessionID
	_ = instanceID
	_ = projectID
	_ = nrfloPath
	userHome := userCodexHome()

	userTOML, _ := os.ReadFile(filepath.Join(userHome, "config.toml"))
	cleaned := stripHookTables(userTOML)
	configTOML := string(cleaned)
	if !strings.HasSuffix(configTOML, "\n") && configTOML != "" {
		configTOML += "\n"
	}
	if !hasFeaturesTable(cleaned) {
		configTOML += "\n[features]\ncodex_hooks = true\n"
	}
	if workDir != "" && !hasProjectTrust(cleaned, workDir) {
		configTOML += fmt.Sprintf("\n[projects.%q]\ntrust_level = \"trusted\"\n", workDir)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(configTOML), 0o644); err != nil {
		return err
	}

	if authBytes, err := os.ReadFile(filepath.Join(userHome, "auth.json")); err == nil {
		_ = os.WriteFile(filepath.Join(dir, "auth.json"), authBytes, 0o600)
	}
	return nil
}

// stripHookTables removes every `[[hooks.<...>]]` array-of-tables block from
// the user's config so only our `-c`-injected hooks fire. A block runs from
// its `[[hooks.…]]` header through the line before the next top-level `[` or
// `[[…]]` header (or EOF).
func stripHookTables(toml []byte) []byte {
	lines := strings.Split(string(toml), "\n")
	out := make([]string, 0, len(lines))
	skipping := false
	for _, raw := range lines {
		trimmed := strings.TrimSpace(raw)
		isHeader := strings.HasPrefix(trimmed, "[")
		if isHeader {
			skipping = strings.HasPrefix(trimmed, "[[hooks.") || strings.HasPrefix(trimmed, "[hooks.") || trimmed == "[hooks]"
		}
		if !skipping {
			out = append(out, raw)
		}
	}
	return []byte(strings.Join(out, "\n"))
}

// hasProjectTrust reports whether toml content already declares a trust entry
// for the given path (`[projects."<path>"]`).
func hasProjectTrust(toml []byte, path string) bool {
	needle := fmt.Sprintf("[projects.%q]", path)
	return strings.Contains(string(toml), needle)
}

// hasFeaturesTable reports whether toml content already opens a top-level
// `[features]` table.
func hasFeaturesTable(toml []byte) bool {
	for _, raw := range strings.Split(string(toml), "\n") {
		line := strings.TrimSpace(raw)
		if line == "[features]" {
			return true
		}
	}
	return false
}

// buildCodexHookCommand assembles the hook command string used in inline
// `-c hooks.<event>=…` flags. Codex strips most env vars from hook
// subprocesses (only CODEX_HOME, HOME, HOMEBREW_*, SHELL, TMPDIR, USER, PATH
// survive); the `/usr/bin/env <vars> nrflo …` wrapper guarantees nrflo CLI
// sees NRF_SESSION_ID/NRF_WORKFLOW_INSTANCE_ID/NRFLO_PROJECT regardless.
// sessionID/instanceID/projectID may be empty for tests.
func buildCodexHookCommand(nrfloPath, sessionID, instanceID, projectID string) string {
	parts := []string{"/usr/bin/env"}
	if sessionID != "" {
		parts = append(parts, "NRF_SESSION_ID="+sessionID)
	}
	if instanceID != "" {
		parts = append(parts, "NRF_WORKFLOW_INSTANCE_ID="+instanceID)
	}
	if projectID != "" {
		parts = append(parts, "NRFLO_PROJECT="+projectID)
	}
	parts = append(parts, nrfloPath, "agent", "record-event")
	return strings.Join(parts, " ")
}

func userCodexHome() string {
	if home := os.Getenv("HOME"); home != "" {
		return filepath.Join(home, ".codex")
	}
	return ".codex"
}

// codexHookEvents is the canonical list of codex hook events the spawner wires
// up for interactive sessions. Used by CodexAdapter.PrepareInteractive.
var codexHookEvents = []string{"PreToolUse", "PostToolUse", "SessionStart", "UserPromptSubmit", "Stop"}
