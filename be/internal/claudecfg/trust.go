// Package claudecfg edits Claude Code's global config file (.claude.json).
package claudecfg

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// claudeTrustMu serializes this process's read-modify-write of .claude.json.
var claudeTrustMu sync.Mutex

// TrustFolder pre-accepts Claude Code's folder-trust dialog for
// dir. The dialog appears even under --dangerously-skip-permissions and its
// default is "No, exit", so the prompt's submitting Enter would exit the CLI.
func TrustFolder(env []string, dir string) error {
	if dir == "" {
		return nil
	}
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		dir = resolved
	}
	path := claudeGlobalConfigPath(env)
	if path == "" || (testing.Testing() && !underTempDir(path)) {
		return nil
	}

	claudeTrustMu.Lock()
	defer claudeTrustMu.Unlock()

	config := map[string]json.RawMessage{}
	mode := os.FileMode(0o600)
	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		if err := json.Unmarshal(data, &config); err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
		if info, statErr := os.Stat(path); statErr == nil {
			mode = info.Mode().Perm()
		}
	case os.IsNotExist(err):
	default:
		return err
	}

	projects := map[string]map[string]json.RawMessage{}
	if raw, ok := config["projects"]; ok {
		if err := json.Unmarshal(raw, &projects); err != nil {
			return fmt.Errorf("parse %s projects: %w", path, err)
		}
	}
	entry := projects[dir]
	if string(entry["hasTrustDialogAccepted"]) == "true" {
		return nil
	}
	if entry == nil {
		entry = map[string]json.RawMessage{}
	}
	entry["hasTrustDialogAccepted"] = json.RawMessage("true")
	projects[dir] = entry
	rawProjects, err := json.Marshal(projects)
	if err != nil {
		return err
	}
	config["projects"] = rawProjects
	out, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(path, append(out, '\n'), mode)
}

// claudeGlobalConfigPath mirrors Claude Code's lookup: $CLAUDE_CONFIG_DIR/.claude.json
// when set, else $HOME/.claude.json.
func claudeGlobalConfigPath(env []string) string {
	if dir := envOrProcess(env, "CLAUDE_CONFIG_DIR"); dir != "" {
		return filepath.Join(dir, ".claude.json")
	}
	home := envOrProcess(env, "HOME")
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	if home == "" {
		return ""
	}
	return filepath.Join(home, ".claude.json")
}

func envOrProcess(env []string, key string) string {
	prefix := key + "="
	for _, e := range env {
		if strings.HasPrefix(e, prefix) {
			return e[len(prefix):]
		}
	}
	return os.Getenv(key)
}

func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), mode); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// underTempDir guards test binaries from writing a developer's real config.
func underTempDir(path string) bool {
	tmp, err := filepath.EvalSymlinks(os.TempDir())
	if err != nil {
		return false
	}
	if resolved, err := filepath.EvalSymlinks(filepath.Dir(path)); err == nil {
		path = filepath.Join(resolved, filepath.Base(path))
	}
	rel, err := filepath.Rel(tmp, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, "../")
}
