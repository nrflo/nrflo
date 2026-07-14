package spawner

import "encoding/json"

// consolePreToolUseTimeoutSec is the per-hook `timeout` (seconds) the CLI
// applies to the PreToolUse hook command. It must exceed
// consoleApprovalTimeout (600s) plus the record-event --console deadline
// (630s) so the blocking approval call is never killed mid-wait — see the
// timeout ladder in REFERENCE.md.
const consolePreToolUseTimeoutSec = 660

// BuildConsoleSettingsJSON returns the Claude --settings JSON for a console
// (human-attended) session: the same hook event set as
// BuildInteractiveSettingsJSON, but every hook command runs `agent
// record-event --console` and the PreToolUse entry carries a numeric
// `timeout` (seconds) long enough to survive a blocking human approval wait.
// Never merged with BuildSafetySettingsJSON — console sessions get no
// safety-hook injection.
func BuildConsoleSettingsJSON(nrfloPath string) string {
	command := nrfloPath + " agent record-event --console"

	hookEntry := func(timeoutSec int) map[string]interface{} {
		hook := map[string]interface{}{
			"type":    "command",
			"command": command,
		}
		if timeoutSec > 0 {
			hook["timeout"] = timeoutSec
		}
		return map[string]interface{}{
			"matcher": "*",
			"hooks":   []interface{}{hook},
		}
	}

	settings := map[string]interface{}{
		"hooks": map[string]interface{}{
			"PreToolUse":       []interface{}{hookEntry(consolePreToolUseTimeoutSec)},
			"PostToolUse":      []interface{}{hookEntry(0)},
			"UserPromptSubmit": []interface{}{hookEntry(0)},
			"Notification":     []interface{}{hookEntry(0)},
			"SubagentStop":     []interface{}{hookEntry(0)},
			"PreCompact":       []interface{}{hookEntry(0)},
			"Stop":             []interface{}{hookEntry(0)},
			"SessionStart":     []interface{}{hookEntry(0)},
		},
		"statusLine": map[string]interface{}{
			"type":    "command",
			"command": nrfloPath + " agent statusline",
		},
	}

	out, err := json.Marshal(settings)
	if err != nil {
		return ""
	}
	return string(out)
}
