package spawner

import (
	"encoding/json"
	"os"
	"sync"
)

var (
	nrfloPathOnce sync.Once
	nrfloPath     string
)

// resolvedNrfloPath returns the absolute path of the current executable,
// which is the nrflo binary when running as a spawned hook. Falls back to
// the literal "nrflo" if os.Executable() returns an error.
func resolvedNrfloPath() string {
	nrfloPathOnce.Do(func() {
		if exe, err := os.Executable(); err == nil {
			nrfloPath = exe
		} else {
			nrfloPath = "nrflo"
		}
	})
	return nrfloPath
}

// BuildInteractiveSettingsJSON returns a Claude --settings JSON string that
// registers PreToolUse and PostToolUse hooks. Each hook pipes the Claude-supplied
// JSON payload to `nrflo agent record-event` via stdin so the server can record
// tool events and reset stall detection. Returns "" for non-Claude agents.
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

	settings := map[string]interface{}{
		"hooks": map[string]interface{}{
			"PreToolUse":  []interface{}{hookEntry},
			"PostToolUse": []interface{}{hookEntry},
		},
	}

	out, err := json.Marshal(settings)
	if err != nil {
		return ""
	}
	return string(out)
}
