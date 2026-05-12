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
	// Workdir trust is delivered as a `-c projects.<dir>.trust_level="trusted"`
	// override in BuildInteractiveCommand. Writing it to <CODEX_HOME>/config.toml
	// is a no-op in codex 0.130 — that file is consulted for hooks but not for
	// trust resolution; trust always reads ~/.codex/config.toml. See
	// cli_adapter_codex.go BuildInteractiveCommand for the -c trust override.
	_ = workDir
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

// ensureCodexUserTrust idempotently appends `[projects."<resolvedPath>"]
// trust_level = "trusted"` to `~/.codex/config.toml`. Codex 0.130 reads trust
// ONLY from this file (the per-session CODEX_HOME and `-c` overrides for the
// `projects."<path>".trust_level` nested key are both silently ignored), and
// the trust prompt blocks even under `--dangerously-bypass-approvals-and-sandbox`.
//
// Path is symlink-resolved (e.g. `/var/folders` → `/private/var/folders` on
// macOS) so the key matches what codex's own cwd canonicalization writes when
// a user clicks "yes". If `~/.codex/config.toml` already contains a trust
// entry for the resolved path, this is a no-op and returns `added=false`.
// Otherwise the entry is appended and `added=true` is returned so the caller
// can clean it up on session end (avoids unbounded config bloat for ephemeral
// workdirs like CI/harness tempdirs).
//
// Concurrency: best-effort. Two spawners racing on the same workdir could both
// append; codex's TOML parser tolerates duplicate `[projects."<path>"]` tables
// (last one wins with the same value either way). The cleanup path is also
// idempotent — it removes one matching block per call.
func ensureCodexUserTrust(workDir string) (added bool, resolved string, err error) {
	resolved, evalErr := filepath.EvalSymlinks(workDir)
	if evalErr != nil {
		resolved = workDir
	}

	userHome := userCodexHome()
	if err := os.MkdirAll(userHome, 0o700); err != nil {
		return false, resolved, fmt.Errorf("mkdir %s: %w", userHome, err)
	}
	configPath := filepath.Join(userHome, "config.toml")

	existing, _ := os.ReadFile(configPath) // missing file is fine — empty content
	if hasProjectTrust(existing, resolved) {
		return false, resolved, nil
	}

	entry := fmt.Sprintf("\n[projects.%q]\ntrust_level = \"trusted\"\n", resolved)
	body := string(existing)
	if body != "" && !strings.HasSuffix(body, "\n") {
		body += "\n"
	}
	body += entry
	if err := os.WriteFile(configPath, []byte(body), 0o600); err != nil {
		return false, resolved, err
	}
	return true, resolved, nil
}

// removeCodexUserTrust strips the `[projects."<resolvedPath>"]` block we
// appended in ensureCodexUserTrust from `~/.codex/config.toml`. Only the block
// we wrote (header line + trust_level line + the leading blank we prepended)
// is removed; other content is preserved. Idempotent — a no-op when the file
// is missing or the entry is already gone.
//
// We only call this for entries WE appended (tracked via the `added` return
// from ensureCodexUserTrust). Pre-existing entries (user said "yes" once for
// a real project) stay put.
func removeCodexUserTrust(resolvedPath string) error {
	userHome := userCodexHome()
	configPath := filepath.Join(userHome, "config.toml")
	existing, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	header := fmt.Sprintf("[projects.%q]", resolvedPath)
	lines := strings.Split(string(existing), "\n")
	out := make([]string, 0, len(lines))
	skip := false
	skippedAny := false
	for _, ln := range lines {
		if skip {
			// Skip the `trust_level = "trusted"` line that follows the header.
			// Bail out of skip mode on the next blank line or next table header.
			trimmed := strings.TrimSpace(ln)
			if trimmed == "" || strings.HasPrefix(trimmed, "[") {
				skip = false
				if trimmed == "" {
					// Drop the trailing blank we added with the entry.
					continue
				}
				// Don't drop the next table header; fall through.
			} else {
				continue // drop trust_level line
			}
		}
		if strings.TrimSpace(ln) == header {
			skip = true
			skippedAny = true
			// Drop the leading blank we prepended (if previous emitted line is blank).
			if len(out) > 0 && strings.TrimSpace(out[len(out)-1]) == "" {
				out = out[:len(out)-1]
			}
			continue
		}
		out = append(out, ln)
	}
	if !skippedAny {
		return nil
	}
	return os.WriteFile(configPath, []byte(strings.Join(out, "\n")), 0o600)
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
