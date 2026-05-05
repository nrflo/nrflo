package pythonsdk

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestPythonSDK_UnitTests runs test_nrflo_sdk.py via python3 -m unittest.
// The test is skipped when python3 is not available so CI on Python-free hosts
// is not blocked.
func TestPythonSDK_UnitTests(t *testing.T) {
	python3, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 not found in PATH — skipping Python SDK unit tests")
	}

	dir := t.TempDir()

	if err := WriteSDK(dir); err != nil {
		t.Fatalf("WriteSDK: %v", err)
	}

	// Copy test_nrflo_sdk.py next to the SDK so the test can import it.
	srcPath := filepath.Join("test_nrflo_sdk.py")
	data, err := os.ReadFile(srcPath)
	if err != nil {
		t.Fatalf("read test_nrflo_sdk.py: %v", err)
	}
	dstPath := filepath.Join(dir, "test_nrflo_sdk.py")
	if err := os.WriteFile(dstPath, data, 0o644); err != nil {
		t.Fatalf("write test file to temp dir: %v", err)
	}

	cmd := exec.Command(python3, "-m", "unittest", "test_nrflo_sdk", "-v")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Errorf("python3 -m unittest failed:\n%s", out)
	} else {
		t.Logf("python unittest output:\n%s", out)
	}
}
