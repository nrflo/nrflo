package claudecfg

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func readClaudeProjects(t *testing.T, path string) map[string]map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var config struct {
		Projects map[string]map[string]any `json:"projects"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return config.Projects
}

func TestTrustFolder_CreatesConfig(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	home := t.TempDir()
	work := t.TempDir()
	if err := TrustFolder([]string{"HOME=" + home}, work); err != nil {
		t.Fatalf("TrustFolder: %v", err)
	}
	resolved, _ := filepath.EvalSymlinks(work)
	projects := readClaudeProjects(t, filepath.Join(home, ".claude.json"))
	if projects[resolved]["hasTrustDialogAccepted"] != true {
		t.Fatalf("projects[%q] = %v, want hasTrustDialogAccepted=true", resolved, projects[resolved])
	}
}

func TestTrustFolder_PreservesExistingKeys(t *testing.T) {
	configDir := t.TempDir()
	work := t.TempDir()
	resolved, _ := filepath.EvalSymlinks(work)
	path := filepath.Join(configDir, ".claude.json")
	seed := `{"hasCompletedOnboarding":true,"projects":{"` + resolved + `":{"allowedTools":["x"]},"/other":{"lastCost":1}}}`
	if err := os.WriteFile(path, []byte(seed), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := TrustFolder([]string{"CLAUDE_CONFIG_DIR=" + configDir}, work); err != nil {
		t.Fatalf("TrustFolder: %v", err)
	}

	data, _ := os.ReadFile(path)
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	if config["hasCompletedOnboarding"] != true {
		t.Errorf("top-level key lost: %s", data)
	}
	projects := readClaudeProjects(t, path)
	if projects[resolved]["hasTrustDialogAccepted"] != true || projects[resolved]["allowedTools"] == nil {
		t.Errorf("projects[%q] = %v, want trust added and allowedTools kept", resolved, projects[resolved])
	}
	if projects["/other"]["lastCost"] != float64(1) {
		t.Errorf("unrelated project entry changed: %v", projects["/other"])
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o640 {
		t.Errorf("mode = %v, want 0640", info.Mode().Perm())
	}
}

func TestTrustFolder_TestBinaryNeverWritesOutsideTempDir(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	home := "/nonexistent-nrflo-home"
	if err := TrustFolder([]string{"HOME=" + home}, t.TempDir()); err != nil {
		t.Fatalf("TrustFolder: %v", err)
	}
	if _, err := os.Stat(home); !os.IsNotExist(err) {
		t.Fatalf("config dir outside temp was touched: %v", err)
	}
}
