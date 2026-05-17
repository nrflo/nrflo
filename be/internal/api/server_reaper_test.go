package api

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"be/internal/clock"
)

// TestReapStaleUploads verifies that reapStaleUploads removes dirs older than 1h
// and leaves fresh dirs untouched.
func TestReapStaleUploads(t *testing.T) {
	s, dir := newArtifactHandlerServer(t)

	// Staging root is <dir>/tmp/uploads (relative to dataPath's parent which is dir)
	stagingRoot := filepath.Join(dir, "tmp", "uploads")
	if err := os.MkdirAll(stagingRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll staging root: %v", err)
	}

	// Create a fresh directory.
	freshDir := filepath.Join(stagingRoot, "fresh-upload-id")
	if err := os.Mkdir(freshDir, 0o755); err != nil {
		t.Fatalf("mkdir fresh: %v", err)
	}

	// Create a stale directory with a ModTime in the past.
	staleDir := filepath.Join(stagingRoot, "stale-upload-id")
	if err := os.Mkdir(staleDir, 0o755); err != nil {
		t.Fatalf("mkdir stale: %v", err)
	}
	staleTime := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(staleDir, staleTime, staleTime); err != nil {
		t.Fatalf("chtimes stale: %v", err)
	}

	// Replace server clock with real (clock.Now() must be > cutoff for stale).
	s.clock = clock.Real()
	s.reapStaleUploads()

	// Fresh dir should survive.
	if _, err := os.Stat(freshDir); err != nil {
		t.Errorf("fresh dir should still exist: %v", err)
	}

	// Stale dir should be gone.
	if _, err := os.Stat(staleDir); !os.IsNotExist(err) {
		t.Error("stale dir should have been removed")
	}
}

// TestReapStaleUploads_MissingRoot is a no-op when staging root does not exist.
func TestReapStaleUploads_MissingRoot(t *testing.T) {
	dir := t.TempDir()
	dataPath := filepath.Join(dir, "nrflo.data")
	s := &Server{dataPath: dataPath, clock: clock.Real()}
	// No staging root created — should not panic.
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("reapStaleUploads panicked: %v", r)
		}
	}()
	s.reapStaleUploads()
}

// TestReapStaleUploads_EmptyDataPath verifies no-op when dataPath is unset.
func TestReapStaleUploads_EmptyDataPath(t *testing.T) {
	s := &Server{dataPath: "", clock: clock.Real()}
	s.reapStaleUploads() // must not panic
}
