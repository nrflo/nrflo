package spawner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

type geminiHook struct {
	Command string `json:"command"`
	Timeout int    `json:"timeout"`
}

type geminiSettings struct {
	Hooks map[string][]geminiHook `json:"hooks"`
}

// geminiHookEvents is the canonical list of Gemini hook events the spawner wires
// up for interactive sessions.
var geminiHookEvents = []string{"BeforeTool", "AfterTool", "AfterAgent", "SessionStart", "SessionEnd", "Notification"}

// prepareGeminiHome creates a per-session HOME dir with a .gemini subdirectory,
// symlinks user auth files (best-effort), and writes settings.json with hooks.
// Returns the dir path, a cleanup func, and any error.
func prepareGeminiHome(opts InteractivePrepOptions) (string, func(), error) {
	dir, err := os.MkdirTemp("", "nrflo-gemini-"+opts.SessionID+"-*")
	if err != nil {
		return "", func() {}, err
	}
	geminiDir := filepath.Join(dir, ".gemini")
	if err := os.MkdirAll(geminiDir, 0o700); err != nil {
		_ = os.RemoveAll(dir)
		return "", func() {}, err
	}
	userHome := userGeminiHome()
	for _, f := range []string{"oauth_creds.json", "google_accounts.json", "installation_id"} {
		src := filepath.Join(userHome, f)
		dst := filepath.Join(geminiDir, f)
		_ = os.Symlink(src, dst) // best-effort; missing source or permission error is non-fatal
	}
	cmd := buildGeminiHookCommand(resolvedNrfloPath(), opts.SessionID, opts.WorkflowInstanceID, opts.ProjectID)
	hooks := make(map[string][]geminiHook, len(geminiHookEvents))
	for _, ev := range geminiHookEvents {
		hooks[ev] = []geminiHook{{Command: cmd, Timeout: 5}}
	}
	settings := geminiSettings{Hooks: hooks}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		_ = os.RemoveAll(dir)
		return "", func() {}, err
	}
	if err := os.WriteFile(filepath.Join(geminiDir, "settings.json"), data, 0o600); err != nil {
		_ = os.RemoveAll(dir)
		return "", func() {}, err
	}
	return dir, func() { _ = os.RemoveAll(dir) }, nil
}

// buildGeminiHookCommand assembles the hook command string for Gemini hook events.
// Uses /usr/bin/env wrapper to guarantee NRF_*/NRFLO_PROJECT vars reach the nrflo CLI
// regardless of what Gemini strips from hook subprocess environments.
// sessionID/instanceID/projectID may be empty for tests.
func buildGeminiHookCommand(nrfloPath, sessionID, instanceID, projectID string) string {
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

func userGeminiHome() string {
	if home := os.Getenv("HOME"); home != "" {
		return filepath.Join(home, ".gemini")
	}
	return ".gemini"
}
