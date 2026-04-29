package spawner

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// BuildCodexHookProfile creates a temporary CODEX_HOME directory with config.toml
// and hooks.json configured for nrflo telemetry. Returns the dir path, a cleanup
// func, and any error. cleanup calls os.RemoveAll (best-effort).
func BuildCodexHookProfile(proc *processInfo) (dir string, cleanup func(), err error) {
	dir, err = os.MkdirTemp("", "nrflo-codex-"+proc.sessionID+"-*")
	if err != nil {
		return "", func() {}, err
	}
	if err = WriteCodexProfile(dir, resolvedNrfloPath()); err != nil {
		_ = os.RemoveAll(dir)
		return "", func() {}, fmt.Errorf("write codex profile: %w", err)
	}
	return dir, func() { _ = os.RemoveAll(dir) }, nil
}

// WriteCodexProfile writes config.toml and hooks.json into dir.
// config.toml enables codex_hooks; hooks.json registers PreToolUse and PostToolUse
// hooks pointing at `nrfloPath agent record-event`.
func WriteCodexProfile(dir, nrfloPath string) error {
	configTOML := "[features]\ncodex_hooks = true\n"
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(configTOML), 0o644); err != nil {
		return err
	}

	command := nrfloPath + " agent record-event"
	hookEntry := map[string]interface{}{
		"matcher": "*",
		"hooks": []interface{}{
			map[string]interface{}{
				"type":    "command",
				"command": command,
				"timeout": 5,
			},
		},
	}
	hooks := map[string]interface{}{
		"PreToolUse":  []interface{}{hookEntry},
		"PostToolUse": []interface{}{hookEntry},
	}
	data, err := json.Marshal(hooks)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "hooks.json"), data, 0o644)
}
