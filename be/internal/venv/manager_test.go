package venv

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"be/internal/clock"
)

// newMgr builds a Manager pointing at a fresh temp dataDir with a no-op
// lookPath and a "true"-returning cmdFactory.  Tests override those fields
// before calling Ensure.
func newMgr(t *testing.T) (*Manager, string, string) {
	t.Helper()
	dataDir := t.TempDir()
	root := t.TempDir()
	m := New(dataDir, clock.NewTest(time.Now()))
	m.lookPath = func(string) (string, error) { return "/fake/python3", nil }
	m.cmdFactory = func(_ context.Context, _ string, _ ...string) *exec.Cmd {
		return exec.Command("true")
	}
	return m, dataDir, root
}

// stubPython creates a minimal python3 stub inside venvDir/bin so that
// venvExists() returns true for that venv.
func stubPython(t *testing.T, venvDir string) string {
	t.Helper()
	binDir := filepath.Join(venvDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("stubPython mkdir: %v", err)
	}
	py := filepath.Join(binDir, "python3")
	if err := os.WriteFile(py, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("stubPython write: %v", err)
	}
	return py
}

// writeReqs writes content to <dir>/requirements.txt.
func writeReqs(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "requirements.txt"), []byte(content), 0o644); err != nil {
		t.Fatalf("writeReqs: %v", err)
	}
}

// hashOf returns the sha256 hex digest of s.
func hashOf(s string) string {
	sum := sha256.Sum256([]byte(s))
	return fmt.Sprintf("%x", sum)
}

// writeHashFile writes hash to path, creating parent dirs as needed.
func writeHashFile(t *testing.T, path, hash string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("writeHashFile mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(hash), 0o644); err != nil {
		t.Fatalf("writeHashFile: %v", err)
	}
}

// notCalledFactory returns a cmdFactory that marks the test as failed if invoked.
func notCalledFactory(t *testing.T) func(context.Context, string, ...string) *exec.Cmd {
	t.Helper()
	return func(_ context.Context, _ string, _ ...string) *exec.Cmd {
		t.Error("cmdFactory should not have been called")
		return exec.Command("true")
	}
}

// --- Tests ---

func TestEnsure_NoRequirementsNoVenv(t *testing.T) {
	m, _, root := newMgr(t)
	m.cmdFactory = notCalledFactory(t)

	got, err := m.Ensure(context.Background(), "p1", root)
	if err != nil {
		t.Errorf("Ensure() error = %v, want nil", err)
	}
	if got != "" {
		t.Errorf("Ensure() = %q, want empty string", got)
	}
}

func TestEnsure_NoRequirementsExistingVenv(t *testing.T) {
	m, dataDir, root := newMgr(t)
	m.cmdFactory = notCalledFactory(t)

	venvDir := filepath.Join(dataDir, "project", "p1", "venv")
	wantBin := stubPython(t, venvDir)

	got, err := m.Ensure(context.Background(), "p1", root)
	if err != nil {
		t.Errorf("Ensure() error = %v, want nil", err)
	}
	if got != wantBin {
		t.Errorf("Ensure() = %q, want %q", got, wantBin)
	}
}

func TestEnsure_FreshVenvWithRequirements(t *testing.T) {
	m, dataDir, root := newMgr(t)
	const reqContent = "requests==2.28.0\n"
	writeReqs(t, root, reqContent)

	projDir := filepath.Join(dataDir, "project", "p1")
	venvDir := filepath.Join(projDir, "venv")
	pythonBin := filepath.Join(venvDir, "bin", "python3")
	hashFile := filepath.Join(projDir, "requirements.sha256")

	callN := 0
	m.cmdFactory = func(_ context.Context, _ string, _ ...string) *exec.Cmd {
		callN++
		if callN == 1 {
			// Simulate venv creation: install the python3 stub so that
			// venvExists() sees it on subsequent checks.
			_ = os.MkdirAll(filepath.Join(venvDir, "bin"), 0o755)
			_ = os.WriteFile(pythonBin, []byte("#!/bin/sh\n"), 0o755)
		}
		return exec.Command("true")
	}

	got, err := m.Ensure(context.Background(), "p1", root)
	if err != nil {
		t.Errorf("Ensure() error = %v, want nil", err)
	}
	if got != pythonBin {
		t.Errorf("Ensure() = %q, want %q", got, pythonBin)
	}
	if callN != 2 {
		t.Errorf("cmdFactory called %d times, want 2 (venv + pip)", callN)
	}
	hashBytes, readErr := os.ReadFile(hashFile)
	if readErr != nil {
		t.Fatalf("hash file missing after successful run: %v", readErr)
	}
	if string(hashBytes) != hashOf(reqContent) {
		t.Errorf("hash file = %q, want %q", string(hashBytes), hashOf(reqContent))
	}
}

func TestEnsure_HashUnchanged_SkipsPip(t *testing.T) {
	m, dataDir, root := newMgr(t)
	m.cmdFactory = notCalledFactory(t)
	const reqContent = "flask==3.0.0\n"
	writeReqs(t, root, reqContent)

	projDir := filepath.Join(dataDir, "project", "p1")
	venvDir := filepath.Join(projDir, "venv")
	wantBin := stubPython(t, venvDir)
	writeHashFile(t, filepath.Join(projDir, "requirements.sha256"), hashOf(reqContent))

	got, err := m.Ensure(context.Background(), "p1", root)
	if err != nil {
		t.Errorf("Ensure() error = %v, want nil", err)
	}
	if got != wantBin {
		t.Errorf("Ensure() = %q, want %q", got, wantBin)
	}
}

func TestEnsure_HashChanged_ReinstallsPip(t *testing.T) {
	m, dataDir, root := newMgr(t)
	const reqContent = "flask==3.1.0\n"
	writeReqs(t, root, reqContent)

	projDir := filepath.Join(dataDir, "project", "p1")
	venvDir := filepath.Join(projDir, "venv")
	wantBin := stubPython(t, venvDir)
	hashFile := filepath.Join(projDir, "requirements.sha256")
	writeHashFile(t, hashFile, "stale_hash_value")

	callN := 0
	m.cmdFactory = func(_ context.Context, _ string, _ ...string) *exec.Cmd {
		callN++
		return exec.Command("true")
	}

	got, err := m.Ensure(context.Background(), "p1", root)
	if err != nil {
		t.Errorf("Ensure() error = %v, want nil", err)
	}
	if got != wantBin {
		t.Errorf("Ensure() = %q, want %q", got, wantBin)
	}
	if callN != 1 {
		t.Errorf("cmdFactory called %d times, want 1 (pip only, no venv create)", callN)
	}
	hashBytes, readErr := os.ReadFile(hashFile)
	if readErr != nil {
		t.Fatalf("hash file missing: %v", readErr)
	}
	if string(hashBytes) != hashOf(reqContent) {
		t.Errorf("hash = %q, want %q", string(hashBytes), hashOf(reqContent))
	}
}

func TestEnsure_VenvCreateFails(t *testing.T) {
	m, dataDir, root := newMgr(t)
	writeReqs(t, root, "numpy\n")
	hashFile := filepath.Join(dataDir, "project", "p1", "requirements.sha256")

	m.cmdFactory = func(_ context.Context, _ string, _ ...string) *exec.Cmd {
		return exec.Command("false") // venv create fails
	}

	got, err := m.Ensure(context.Background(), "p1", root)
	if err != nil {
		t.Errorf("Ensure() error = %v, want nil", err)
	}
	if got != "" {
		t.Errorf("Ensure() = %q, want empty string", got)
	}
	if _, statErr := os.Stat(hashFile); !os.IsNotExist(statErr) {
		t.Error("hash file must not exist when venv creation fails")
	}
}

func TestEnsure_PipFailsFreshVenv(t *testing.T) {
	m, dataDir, root := newMgr(t)
	const reqContent = "pandas\n"
	writeReqs(t, root, reqContent)

	projDir := filepath.Join(dataDir, "project", "p1")
	venvDir := filepath.Join(projDir, "venv")
	pythonBin := filepath.Join(venvDir, "bin", "python3")
	hashFile := filepath.Join(projDir, "requirements.sha256")

	callN := 0
	m.cmdFactory = func(_ context.Context, _ string, _ ...string) *exec.Cmd {
		callN++
		if callN == 1 {
			_ = os.MkdirAll(filepath.Join(venvDir, "bin"), 0o755)
			_ = os.WriteFile(pythonBin, []byte("#!/bin/sh\n"), 0o755)
			return exec.Command("true") // venv create succeeds
		}
		return exec.Command("false") // pip fails
	}

	got, err := m.Ensure(context.Background(), "p1", root)
	if err != nil {
		t.Errorf("Ensure() error = %v, want nil", err)
	}
	if got != pythonBin {
		t.Errorf("Ensure() = %q, want %q (pythonBin still returned on pip fail)", got, pythonBin)
	}
	if _, statErr := os.Stat(hashFile); !os.IsNotExist(statErr) {
		t.Error("hash file must not exist when pip fails on fresh venv")
	}
}

func TestEnsure_PipFailsExistingVenv(t *testing.T) {
	m, dataDir, root := newMgr(t)
	const reqContent = "scipy\n"
	writeReqs(t, root, reqContent)

	projDir := filepath.Join(dataDir, "project", "p1")
	venvDir := filepath.Join(projDir, "venv")
	wantBin := stubPython(t, venvDir)
	hashFile := filepath.Join(projDir, "requirements.sha256")
	const staleHash = "old_stale_hash"
	writeHashFile(t, hashFile, staleHash)

	m.cmdFactory = func(_ context.Context, _ string, _ ...string) *exec.Cmd {
		return exec.Command("false") // pip update fails
	}

	got, err := m.Ensure(context.Background(), "p1", root)
	if err != nil {
		t.Errorf("Ensure() error = %v, want nil", err)
	}
	if got != wantBin {
		t.Errorf("Ensure() = %q, want %q", got, wantBin)
	}
	hashBytes, readErr := os.ReadFile(hashFile)
	if readErr != nil {
		t.Fatalf("hash file must still exist: %v", readErr)
	}
	if string(hashBytes) != staleHash {
		t.Errorf("stale hash overwritten: got %q, want %q", string(hashBytes), staleHash)
	}
}

func TestEnsure_BootstrapPython3Missing(t *testing.T) {
	m, _, root := newMgr(t)
	writeReqs(t, root, "requests\n")
	m.lookPath = func(string) (string, error) {
		return "", fmt.Errorf("python3 not found in PATH")
	}
	m.cmdFactory = notCalledFactory(t)

	got, err := m.Ensure(context.Background(), "p1", root)
	if err != nil {
		t.Errorf("Ensure() error = %v, want nil", err)
	}
	if got != "" {
		t.Errorf("Ensure() = %q, want empty string", got)
	}
}

func TestEnsure_RequirementsTxtUnreadable(t *testing.T) {
	m, _, root := newMgr(t)
	m.cmdFactory = notCalledFactory(t)

	// Place a directory at requirements.txt so os.ReadFile fails.
	reqPath := filepath.Join(root, "requirements.txt")
	if err := os.Mkdir(reqPath, 0o755); err != nil {
		t.Fatalf("mkdir requirements.txt: %v", err)
	}

	got, err := m.Ensure(context.Background(), "p1", root)
	if err != nil {
		t.Errorf("Ensure() error = %v, want nil", err)
	}
	if got != "" {
		t.Errorf("Ensure() = %q, want empty string", got)
	}
}
