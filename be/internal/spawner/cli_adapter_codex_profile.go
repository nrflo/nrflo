package spawner

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Codex per-session CODEX_HOME profile. This writer stays hook-free: hooks
// moved to CODEX_HOME/hooks.json around codex 0.140 (see
// hooks_settings_codex.go, used only by the PTY user-session paths), so the
// config.toml `[hooks.*]` tables copied from the user's config now carry only
// hook state/trust, not hook definitions. Because this writer is shared by
// the app-server backend (codex_appserver_backend.go) and the console engine
// (WriteConsoleCodexProfile), keeping it hook-free means those profiles never
// gain hooks. The profile's other job is to keep the agent logged in
// (auth.json) and to grant workdir trust — codex reads the
// `[projects."<path>"] trust_level="trusted"` entry from CODEX_HOME/config.toml,
// and without it the TUI blocks on a directory-trust dialog even under
// `--dangerously-bypass-approvals-and-sandbox`.

// codexStripTablePrefixes are the config.toml table headers dropped when
// copying the user's config into the per-session profile:
//   - hooks: hook definitions live in hooks.json now, not config.toml; strip
//     the user's [hooks.*] state/trust tables so they can't collide with ours.
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
	return appendCodexMCPServer(dir, resolvedNrfloPath(), []string{"agent", "mcp"},
		nrfloBridgeEnv(proc.sessionID, proc.workflowInstanceID, proc.projectID))
}

// WriteConsoleCodexProfile writes the per-session CODEX_HOME profile for a
// server-owned Codex console engine: the same trust/auth/hook-stripping profile
// writer as a spawned session, with the nrflo MCP server wired to the
// `agent mcp-external` bridge (not `agent mcp` — a console chat is a human
// session, not a managed one).
func WriteConsoleCodexProfile(dir, workDir, serverPath string, env map[string]string) error {
	if err := writeCodexProfileForSession(dir, workDir); err != nil {
		return err
	}
	return appendCodexMCPServer(dir, serverPath, []string{"agent", "mcp-external"}, env)
}

// appendCodexMCPServer appends an [mcp_servers.nrflo] table (plus an env table)
// to the per-session CODEX_HOME/config.toml so codex serves the nrflo agent
// tools to the model over MCP (codex calls the bridge as `serverPath <args...>`).
// Codex does not forward parent process env to MCP server subprocesses, so the
// session env the bridge needs (NRF_SESSION_ID, socket path, …) is embedded here.
func appendCodexMCPServer(dir, serverPath string, args []string, env map[string]string) error {
	var b strings.Builder
	quotedArgs := make([]string, len(args))
	for i, a := range args {
		quotedArgs[i] = fmt.Sprintf("%q", a)
	}
	fmt.Fprintf(&b, "\n[mcp_servers.nrflo]\ncommand = %q\nargs = [%s]\n", serverPath, strings.Join(quotedArgs, ", "))
	if len(env) > 0 {
		b.WriteString("\n[mcp_servers.nrflo.env]\n")
		keys := make([]string, 0, len(env))
		for k := range env {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(&b, "%s = %q\n", k, env[k])
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
