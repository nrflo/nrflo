package socket

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// shortSocketPath creates a temp dir under /tmp with a short name and returns a socket path in it.
// macOS sun_path limit is 104 bytes; t.TempDir() paths are too long.
func shortSocketPath(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "nrf")
	if err != nil {
		t.Fatalf("create temp socket dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return filepath.Join(dir, "s.sock")
}

// TestBindListener_Success verifies the happy path: listener is returned, path matches env,
// file exists with mode 0600 after bind.
func TestBindListener_Success(t *testing.T) {
	sockPath := shortSocketPath(t)
	t.Setenv("NRFLO_SOCKET", sockPath)

	ln, gotPath, err := BindListener()
	if err != nil {
		t.Fatalf("BindListener() error = %v, want nil", err)
	}
	t.Cleanup(func() {
		ln.Close()
		os.Remove(sockPath)
	})

	if ln == nil {
		t.Fatal("BindListener() returned nil listener")
	}
	if gotPath != sockPath {
		t.Errorf("BindListener() path = %q, want %q", gotPath, sockPath)
	}

	fi, err := os.Stat(sockPath)
	if err != nil {
		t.Fatalf("stat socket file: %v", err)
	}
	if fi.Mode()&0777 != 0600 {
		t.Errorf("socket file mode = %o, want 0600", fi.Mode()&0777)
	}
}

// TestBindListener_DirectoryAtPath verifies that an error is returned when a directory exists
// at the resolved socket path.
func TestBindListener_DirectoryAtPath(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "nrf")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	// Place a directory at the socket path itself.
	sockPath := filepath.Join(dir, "s.sock")
	if err := os.MkdirAll(sockPath, 0755); err != nil {
		t.Fatalf("create dir at socket path: %v", err)
	}
	t.Setenv("NRFLO_SOCKET", sockPath)

	ln, _, err := BindListener()
	if err == nil {
		ln.Close()
		t.Fatal("BindListener() want error for directory at path, got nil")
	}
	if !strings.Contains(err.Error(), "directory") {
		t.Errorf("BindListener() error = %q, want it to contain %q", err.Error(), "directory")
	}
	if !strings.Contains(err.Error(), sockPath) {
		t.Errorf("BindListener() error = %q, want it to contain %q", err.Error(), sockPath)
	}
}

// TestBindListener_ParentDirUnwritable verifies that an error is returned when the parent
// directory is not writable and the socket file does not already exist.
func TestBindListener_ParentDirUnwritable(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("cannot test unwritable directory as root")
	}

	dir, err := os.MkdirTemp("/tmp", "nrf")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	t.Cleanup(func() {
		os.Chmod(dir, 0755)
		os.RemoveAll(dir)
	})

	// Make the directory unwritable so MkdirAll inside it will fail.
	if err := os.Chmod(dir, 0500); err != nil {
		t.Fatalf("chmod dir: %v", err)
	}

	sockPath := filepath.Join(dir, "sub", "s.sock")
	t.Setenv("NRFLO_SOCKET", sockPath)

	ln, _, err := BindListener()
	if err == nil {
		ln.Close()
		t.Fatal("BindListener() want error for unwritable parent dir, got nil")
	}
	if !strings.Contains(err.Error(), dir) {
		t.Errorf("BindListener() error = %q, want it to contain %q", err.Error(), dir)
	}
}

// TestBindListener_StaleSocket verifies that a stale regular file at the socket path is
// removed and BindListener returns a working listener.
func TestBindListener_StaleSocket(t *testing.T) {
	sockPath := shortSocketPath(t)
	t.Setenv("NRFLO_SOCKET", sockPath)

	// Place a stale regular file (simulating a leftover from a previous run).
	if err := os.WriteFile(sockPath, []byte("stale"), 0644); err != nil {
		t.Fatalf("create stale socket file: %v", err)
	}

	ln, gotPath, err := BindListener()
	if err != nil {
		t.Fatalf("BindListener() error = %v, want nil (stale file should be removed)", err)
	}
	t.Cleanup(func() {
		ln.Close()
		os.Remove(sockPath)
	})

	if ln == nil {
		t.Fatal("BindListener() returned nil listener")
	}
	if gotPath != sockPath {
		t.Errorf("BindListener() path = %q, want %q", gotPath, sockPath)
	}

	// Verify we can actually connect to the listener.
	connErrCh := make(chan error, 1)
	go func() {
		c, err := net.Dial("unix", sockPath)
		if err != nil {
			connErrCh <- err
			return
		}
		c.Close()
		connErrCh <- nil
	}()

	// Accept the connection so the goroutine unblocks.
	conn, err := ln.Accept()
	if err != nil {
		t.Fatalf("listener.Accept() error = %v", err)
	}
	conn.Close()

	if err := <-connErrCh; err != nil {
		t.Errorf("dial stale-removed socket: %v", err)
	}
}

// TestBindListener_PathTooLong verifies the pre-flight error for paths exceeding 100 bytes.
func TestBindListener_PathTooLong(t *testing.T) {
	// Construct a path that is definitely >100 bytes.
	longPath := "/tmp/" + strings.Repeat("x", 100) + ".sock"
	t.Setenv("NRFLO_SOCKET", longPath)

	ln, _, err := BindListener()
	if err == nil {
		ln.Close()
		t.Fatal("BindListener() want error for long path, got nil")
	}
	if !strings.Contains(err.Error(), fmt.Sprintf("%d bytes", len(longPath))) {
		t.Errorf("BindListener() error = %q, want byte-count in message", err.Error())
	}
}
