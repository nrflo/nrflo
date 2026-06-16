package spawner

import (
	"encoding/json"
	"os"
)

// resolvedNrfloPath returns the absolute path to the running nrflo_server binary,
// which hosts the agent infrastructure subcommands (record-event, statusline,
// context-update, mcp). Hooks and the MCP bridge are spawned as short-lived
// `nrflo_server agent <cmd>` processes that connect back over the Unix socket.
// Using the absolute executable path (not a bare name) means it resolves even
// when nrflo_server is not on the spawned agent's PATH (e.g. the Docker image).
func resolvedNrfloPath() string {
	if exe, err := os.Executable(); err == nil {
		return exe
	}
	return "nrflo_server"
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
			// Stop drives end-of-turn completion enforcement: the server returns a
			// block decision (carrying a finish-reminder) when an autonomous turn
			// ends without a completion tool. Long-established event — safe to
			// register (handled in handler_record_event's Stop case).
			"Stop": []interface{}{hookEntry},
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
