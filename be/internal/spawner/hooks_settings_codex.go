package spawner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// codexHookEvents are the five codex 0.145 hook events validated to fire
// under a PTY user session with CODEX_HOME/hooks.json +
// --dangerously-bypass-hook-trust. Payloads are byte-compatible with Claude's
// (hook_event_name CamelCase, session_id/cwd/model/transcript_path), so the
// existing `agent record-event` CLI handles them without changes.
var codexHookEvents = []string{
	"SessionStart",
	"UserPromptSubmit",
	"PreToolUse",
	"PostToolUse",
	"Stop",
}

// buildCodexHooksJSON returns the CODEX_HOME/hooks.json contents registering
// `nrflo agent record-event` for every event in codexHookEvents. Only
// type+command are emitted — codex's optional timeout/async/statusMessage
// handler fields are unvalidated, and a rejected hooks.json makes codex
// silently skip all hooks.
func buildCodexHooksJSON() ([]byte, error) {
	hookEntry := map[string]interface{}{
		"matcher": "*",
		"hooks": []interface{}{
			map[string]interface{}{
				"type":    "command",
				"command": shellQuote(resolvedNrfloPath()) + " agent record-event",
			},
		},
	}

	events := make(map[string]interface{}, len(codexHookEvents))
	for _, ev := range codexHookEvents {
		events[ev] = []interface{}{hookEntry}
	}

	return json.Marshal(map[string]interface{}{"hooks": events})
}

// writeCodexHooksForSession writes hooks.json into the per-session CODEX_HOME
// dir. Kept separate from writeCodexProfileForSession, which is shared with
// the app-server backend and the console engine — those profiles must stay
// hook-free.
func writeCodexHooksForSession(dir string) error {
	data, err := buildCodexHooksJSON()
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "hooks.json"), data, 0o644)
}

// shellQuote wraps s in single quotes for codex's `$SHELL -l -c "<command>"`
// hook invocation, so a path containing spaces is passed as one argument.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
