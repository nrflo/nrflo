// Package venv manages per-project Python virtual environments for script-mode agents.
// It ensures a venv at $NRFLO_HOME/project/<projectID>/venv is in sync with the
// project's requirements.txt (hash-keyed, atomic). Failures are non-blocking —
// callers fall back to PATH python3.
package venv

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"be/internal/clock"
	"be/internal/logger"
)

// Manager ensures per-project Python venvs are in sync with requirements.txt.
type Manager struct {
	dataDir    string
	clk        clock.Clock
	lookPath   func(file string) (string, error)
	cmdFactory func(ctx context.Context, name string, args ...string) *exec.Cmd
}

// New creates a Manager. dataDir is the nrflo data directory (e.g. ~/.nrflo).
func New(dataDir string, clk clock.Clock) *Manager {
	return &Manager{
		dataDir: dataDir,
		clk:     clk,
		lookPath: exec.LookPath,
		cmdFactory: func(ctx context.Context, name string, args ...string) *exec.Cmd {
			return exec.CommandContext(ctx, name, args...)
		},
	}
}

// Ensure returns the path to the python binary in the project's venv, creating
// or updating the venv as needed. Returns ("", nil) on any non-fatal error so
// callers can fall back to PATH python3.
//
// Failure semantics:
//   - No requirements.txt + no existing venv → ("", nil)
//   - No requirements.txt + existing venv     → (pythonBin, nil)
//   - requirements.txt present                → ensure venv + pip install if hash changed
//   - venv create fails                       → ("", nil)
//   - pip fails on fresh venv                 → (pythonBin, nil) without writing hash
//   - pip fails on existing venv              → (pythonBin, nil) without updating hash
func (m *Manager) Ensure(ctx context.Context, projectID, projectRoot string) (string, error) {
	projDir := filepath.Join(m.dataDir, "project", projectID)
	venvDir := filepath.Join(projDir, "venv")
	pythonBin := filepath.Join(venvDir, "bin", "python3")
	hashFile := filepath.Join(projDir, "requirements.sha256")
	reqFile := filepath.Join(projectRoot, "requirements.txt")

	// Check if requirements.txt exists.
	if _, err := os.Stat(reqFile); os.IsNotExist(err) {
		if venvExists(pythonBin) {
			return pythonBin, nil
		}
		return "", nil
	} else if err != nil {
		logger.Warn(ctx, "venv: stat requirements.txt failed", "error", err)
		return "", nil
	}

	// Read requirements.txt and compute hash.
	raw, currentHash, err := readReqHash(reqFile)
	if err != nil {
		logger.Warn(ctx, "venv: read requirements.txt failed", "error", err)
		return "", nil
	}

	_ = raw // used only for the hash; pip reads the file directly

	// Create venv if it doesn't exist.
	if !venvExists(pythonBin) {
		bootstrap, err := m.lookPath("python3")
		if err != nil {
			logger.Warn(ctx, "venv: python3 not found in PATH, cannot create venv")
			return "", nil
		}
		if err := os.MkdirAll(projDir, 0o755); err != nil {
			logger.Warn(ctx, "venv: mkdir projDir failed", "error", err)
			return "", nil
		}
		cmd := m.cmdFactory(ctx, bootstrap, "-m", "venv", venvDir)
		if out, err := cmd.CombinedOutput(); err != nil {
			logger.Warn(ctx, "venv: python3 -m venv failed", "error", err, "output", string(out))
			return "", nil
		}
	}

	// Skip pip install if hash unchanged.
	if storedHash, err := os.ReadFile(hashFile); err == nil {
		if string(storedHash) == currentHash && venvExists(pythonBin) {
			return pythonBin, nil
		}
	}

	// Run pip install -r requirements.txt.
	cmd := m.cmdFactory(ctx, pythonBin, "-m", "pip", "install", "-r", reqFile)
	if out, err := cmd.CombinedOutput(); err != nil {
		logger.Warn(ctx, "venv: pip install failed", "error", err, "output", string(out))
		// Return pythonBin but do NOT write the hash so we retry next time.
		return pythonBin, nil
	}

	// Atomically write the hash.
	if err := writeHashAtomic(hashFile, currentHash); err != nil {
		logger.Warn(ctx, "venv: write hash failed", "error", err)
	}

	return pythonBin, nil
}

// readReqHash reads the file at path and returns the raw content and its sha256 hex digest.
func readReqHash(path string) ([]byte, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	sum := sha256.Sum256(data)
	return data, fmt.Sprintf("%x", sum), nil
}

// writeHashAtomic writes hash to path using a tmp file + rename for atomicity.
func writeHashAtomic(path, hash string) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(hash), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// venvExists reports whether the python binary at pythonBin exists as a regular file.
func venvExists(pythonBin string) bool {
	info, err := os.Stat(pythonBin)
	return err == nil && info.Mode().IsRegular()
}
