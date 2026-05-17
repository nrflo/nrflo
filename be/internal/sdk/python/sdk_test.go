package pythonsdk

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestPythonSDK_UnitTests runs all test_nrflo_sdk*.py files via python3 -m unittest discover.
// The test is skipped when python3 is not available so CI on Python-free hosts is not blocked.
func TestPythonSDK_UnitTests(t *testing.T) {
	python3, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 not found in PATH — skipping Python SDK unit tests")
	}

	dir := t.TempDir()

	if err := WriteSDK(dir); err != nil {
		t.Fatalf("WriteSDK: %v", err)
	}

	testFiles := []string{
		"test_nrflo_sdk.py",
		"test_nrflo_sdk_artifacts.py",
		"test_nrflo_sdk_notification.py",
	}
	for _, f := range testFiles {
		data, err := os.ReadFile(filepath.Join(".", f))
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		if err := os.WriteFile(filepath.Join(dir, f), data, 0o644); err != nil {
			t.Fatalf("write %s to temp dir: %v", f, err)
		}
	}

	cmd := exec.Command(python3, "-m", "unittest", "discover", "-s", ".", "-p", "test_nrflo_sdk*.py", "-v")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Errorf("python3 -m unittest failed:\n%s", out)
	} else {
		t.Logf("python unittest output:\n%s", out)
	}
}
