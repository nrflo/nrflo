package spawner

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
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

// codexStripTablePrefixes are the config.toml table headers dropped when
// copying the user's config into the per-session profile:
//   - hooks: a user's own hook definitions trip codex 0.133's hooks-review gate.
//   - projects: the user's accumulated trust entries (hundreds, often including
//     the spawn workdir) would collide with the single `[projects."<workDir>"]`
//     entry we append — the app-server parses config.toml strictly and rejects
//     duplicate keys (rpc -32600), unlike the lenient TUI.
var codexStripTablePrefixes = []string{
	"[[hooks.", "[hooks.", "[hooks]",
	"[[projects.", "[projects.", "[projects]",
}

// writeCodexProfileForSession writes CODEX_HOME/config.toml and copies the
// user's ~/.codex/auth.json (when present) so the agent stays logged in. The
// user's config.toml is preserved with all hook and project tables stripped
// (see codexStripTablePrefixes), and a single `[projects."<resolvedWorkDir>"]`
// trust entry is appended — so the profile has exactly one project table and
// can't produce a duplicate-key error. workDir is symlink-resolved to match
// codex's cwd canonicalization (e.g. `/var/folders` → `/private/var/folders`).
func writeCodexProfileForSession(dir, workDir string) error {
	userHome := userCodexHome()

	userTOML, _ := os.ReadFile(filepath.Join(userHome, "config.toml"))
	configTOML := string(stripTOMLTables(userTOML, codexStripTablePrefixes))
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

// writeCodexSessionProfile writes the per-session CODEX_HOME profile and, when a
// tool registry was attached to proc (prepareSpawn cli tail), appends the
// [mcp_servers.nrflo] table so codex serves the nrflo agent tools over MCP. The
// bridge env is embedded because codex does not forward parent env to MCP
// subprocesses.
func writeCodexSessionProfile(dir string, proc *processInfo) error {
	if err := writeCodexProfileForSession(dir, proc.workDir); err != nil {
		return err
	}
	if len(proc.apiTools) == 0 {
		return nil
	}
	return appendCodexMCPServer(dir, resolvedNrfloPath(),
		nrfloBridgeEnv(proc.sessionID, proc.workflowInstanceID, proc.projectID))
}

// appendCodexMCPServer appends an [mcp_servers.nrflo] table (plus an env table)
// to the per-session CODEX_HOME/config.toml so codex serves the nrflo agent
// tools to the model over MCP (codex calls the bridge as `serverPath agent mcp`).
// Codex does not forward parent process env to MCP server subprocesses, so the
// session env the bridge needs (NRF_SESSION_ID, socket path, …) is embedded here.
func appendCodexMCPServer(dir, serverPath string, env map[string]string) error {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("\n[mcp_servers.nrflo]\ncommand = %q\nargs = [\"agent\", \"mcp\"]\n", serverPath))
	if len(env) > 0 {
		b.WriteString("\n[mcp_servers.nrflo.env]\n")
		keys := make([]string, 0, len(env))
		for k := range env {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			b.WriteString(fmt.Sprintf("%s = %q\n", k, env[k]))
		}
	}
	f, err := os.OpenFile(filepath.Join(dir, "config.toml"), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(b.String())
	return err
}

// stripTOMLTables removes every table block whose header line begins with any
// of the given prefixes. A block runs from its header through the line before
// the next top-level `[`/`[[…]]` header (or EOF).
func stripTOMLTables(toml []byte, headerPrefixes []string) []byte {
	lines := strings.Split(string(toml), "\n")
	out := make([]string, 0, len(lines))
	skipping := false
	for _, raw := range lines {
		trimmed := strings.TrimSpace(raw)
		if strings.HasPrefix(trimmed, "[") {
			skipping = false
			for _, p := range headerPrefixes {
				if strings.HasPrefix(trimmed, p) {
					skipping = true
					break
				}
			}
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
