package spawner

import (
	"encoding/json"
)

// resolvedNrfloPath returns the literal "nrflo" so spawned agents resolve the
// CLI binary via PATH. Using os.Executable() here was wrong: this code runs
// inside nrflo_server, so the absolute path pointed at nrflo_server (which has
// no `agent` subcommand) and broke every PreToolUse hook.
func resolvedNrfloPath() string {
	return "nrflo"
}

// BuildInteractiveSettingsJSON returns a Claude --settings JSON string that
// registers hooks for every Claude event type we record. Each hook pipes the
// Claude-supplied JSON payload to `nrflo agent record-event` via stdin so the
// server can record tool events, prompts, notifications, and turn boundaries
// (and reset stall detection). Returns "" for non-Claude agents.
func BuildInteractiveSettingsJSON(proc *processInfo) string {
	cliName, _ := parseModelID(proc.modelID)
	if cliName != "claude" {
		return ""
	}

	command := resolvedNrfloPath() + " agent record-event"

	hookEntry := map[string]interface{}{
		"matcher": "*",
		"hooks": []interface{}{
			map[string]interface{}{
				"type":    "command",
				"command": command,
			},
		},
	}

	// Conservative hook set: only events the running Claude CLI version is
	// guaranteed to recognize. Adding unknown keys (e.g. PostToolUseFailure,
	// StopFailure, SubagentStart, UserPromptExpansion) caused the CLI to
	// reject --settings on bootstrap, breaking prompt delivery. Re-add new
	// hooks one at a time after verifying the installed CLI accepts them.
	settings := map[string]interface{}{
		"hooks": map[string]interface{}{
			"PreToolUse":       []interface{}{hookEntry},
			"PostToolUse":      []interface{}{hookEntry},
			"UserPromptSubmit": []interface{}{hookEntry},
			"Notification":     []interface{}{hookEntry},
			"SubagentStop":     []interface{}{hookEntry},
			"PreCompact":       []interface{}{hookEntry},
			// SessionStart is used as a TUI-ready signal (no message
			// recorded). Spawner waits on this before writing the prompt.
			"SessionStart": []interface{}{hookEntry},
		},
		"statusLine": map[string]interface{}{
			"type":    "command",
			"command": resolvedNrfloPath() + " agent statusline",
		},
	}

	out, err := json.Marshal(settings)
	if err != nil {
		return ""
	}
	return string(out)
}
