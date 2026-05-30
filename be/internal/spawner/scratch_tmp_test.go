package spawner

import (
	"os"
	"path/filepath"
	"testing"
)

// TestCreateScratchTemp_CreatesMissingDir is the regression guard for the
// cold-start spawn failure: os.CreateTemp does not create parent dirs, so the
// first spawn on a machine where the scratch dir does not yet exist failed with
// "no such file or directory". createTempInDir must MkdirAll first.
func TestCreateScratchTemp_CreatesMissingDir(t *testing.T) {
	t.Parallel()

	// A guaranteed-absent, multi-level dir under the test's temp root.
	dir := filepath.Join(t.TempDir(), "missing", "nrflo", "scratch")
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("precondition: dir should not exist yet, stat err = %v", err)
	}

	f, err := createTempInDir(dir, "prompt-*.md")
	if err != nil {
		t.Fatalf("createTempInDir on a missing dir = %v, want nil", err)
	}
	t.Cleanup(func() { f.Close(); os.Remove(f.Name()) })

	if got := filepath.Dir(f.Name()); got != dir {
		t.Fatalf("temp file parent = %q, want %q", got, dir)
	}
	if _, err := f.WriteString("hello"); err != nil {
		t.Fatalf("write to scratch temp file = %v, want nil", err)
	}
}

// TestCreateScratchTemp_Idempotent confirms a second call against an existing
// dir still succeeds (self-healing path is a no-op when the dir is present).
func TestCreateScratchTemp_Idempotent(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "scratch")
	for i := 0; i < 2; i++ {
		f, err := createTempInDir(dir, "x-*.md")
		if err != nil {
			t.Fatalf("call %d: createTempInDir = %v, want nil", i, err)
		}
		f.Close()
		os.Remove(f.Name())
	}
}
