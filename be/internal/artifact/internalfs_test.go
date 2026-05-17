package artifact

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestNewFromProjectConfig(t *testing.T) {
	t.Setenv("NRFLO_HOME", t.TempDir())
	ctx := context.Background()

	tests := []struct {
		name    string
		mode    StorageMode
		wantErr bool
	}{
		{"internal", ModeInternal, false},
		{"s3", ModeS3, true},
		{"r2", ModeR2, true},
		{"unknown", StorageMode("badmode"), true},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			s, err := NewFromProjectConfig(ctx, "proj-cfg", Config{Mode: tc.mode})
			if tc.wantErr {
				if err == nil {
					t.Errorf("NewFromProjectConfig(%s) expected error, got nil", tc.mode)
				}
			} else {
				if err != nil {
					t.Errorf("NewFromProjectConfig(%s) unexpected error: %v", tc.mode, err)
				}
				if s == nil {
					t.Error("got nil storage")
				}
			}
		})
	}
}

func TestInternalFS_PutGet(t *testing.T) {
	home := t.TempDir()
	t.Setenv("NRFLO_HOME", home)

	const pid = "proj-ifs"
	const key = "wfi-abc/art-123__report.txt"
	fs := newInternalFS(pid)
	ctx := context.Background()

	content := []byte("hello artifact")
	if err := fs.Put(ctx, key, bytes.NewReader(content)); err != nil {
		t.Fatalf("Put() error: %v", err)
	}

	// Assert on-disk path layout: <home>/projects/<pid>/artifacts/<key>
	wantPath := filepath.Join(home, "projects", pid, "artifacts", "wfi-abc", "art-123__report.txt")
	if _, err := os.Stat(wantPath); err != nil {
		t.Errorf("file not at expected path %s: %v", wantPath, err)
	}

	rc, err := fs.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	defer rc.Close()
	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Errorf("Get() content = %q, want %q", got, content)
	}
}

func TestInternalFS_GetMissing(t *testing.T) {
	t.Setenv("NRFLO_HOME", t.TempDir())
	fs := newInternalFS("proj-gm")
	_, err := fs.Get(context.Background(), "wfi-x/nonexistent__file.txt")
	if err == nil {
		t.Error("Get() missing key expected error, got nil")
	}
}

func TestInternalFS_DeleteEmptiesParent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("NRFLO_HOME", home)

	const pid = "proj-del"
	const key = "wfi-del/art-del__file.txt"
	fs := newInternalFS(pid)
	ctx := context.Background()

	if err := fs.Put(ctx, key, strings.NewReader("data")); err != nil {
		t.Fatalf("Put(): %v", err)
	}
	if err := fs.Delete(ctx, key); err != nil {
		t.Fatalf("Delete(): %v", err)
	}

	// File should be gone.
	fullPath := filepath.Join(home, "projects", pid, "artifacts", "wfi-del", "art-del__file.txt")
	if _, err := os.Stat(fullPath); !os.IsNotExist(err) {
		t.Errorf("file still exists after Delete: %v", err)
	}

	// Empty parent subdir should be removed.
	subdir := filepath.Join(home, "projects", pid, "artifacts", "wfi-del")
	if _, err := os.Stat(subdir); !os.IsNotExist(err) {
		t.Errorf("subdir still exists after Delete (should be emptied): %v", err)
	}

	// artifacts root (the stop boundary) should still exist.
	artifactsRoot := filepath.Join(home, "projects", pid, "artifacts")
	if _, err := os.Stat(artifactsRoot); err != nil {
		t.Errorf("artifacts root removed unexpectedly: %v", err)
	}
}

func TestInternalFS_DeleteStopsAtRoot(t *testing.T) {
	home := t.TempDir()
	t.Setenv("NRFLO_HOME", home)

	const pid = "proj-stop"
	fs := newInternalFS(pid)
	ctx := context.Background()

	// Two sibling files in the same subdir.
	if err := fs.Put(ctx, "wfi-s/art-1__a.txt", strings.NewReader("a")); err != nil {
		t.Fatalf("Put a: %v", err)
	}
	if err := fs.Put(ctx, "wfi-s/art-2__b.txt", strings.NewReader("b")); err != nil {
		t.Fatalf("Put b: %v", err)
	}
	// Delete only one; the subdir is not empty so it should survive.
	if err := fs.Delete(ctx, "wfi-s/art-1__a.txt"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	subdir := filepath.Join(home, "projects", pid, "artifacts", "wfi-s")
	if _, err := os.Stat(subdir); err != nil {
		t.Errorf("subdir removed prematurely (sibling still exists): %v", err)
	}
}

func TestInternalFS_OverwriteAtomic(t *testing.T) {
	home := t.TempDir()
	t.Setenv("NRFLO_HOME", home)

	const pid = "proj-ow"
	const key = "wfi-ow/art-ow__data.bin"
	fs := newInternalFS(pid)
	ctx := context.Background()

	if err := fs.Put(ctx, key, strings.NewReader("original")); err != nil {
		t.Fatalf("Put() first: %v", err)
	}
	if err := fs.Put(ctx, key, strings.NewReader("overwritten")); err != nil {
		t.Fatalf("Put() second: %v", err)
	}

	rc, err := fs.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get(): %v", err)
	}
	defer rc.Close()
	got, _ := io.ReadAll(rc)
	if string(got) != "overwritten" {
		t.Errorf("content = %q, want overwritten", got)
	}

	// No .tmp-* siblings should remain in the parent dir.
	parent := filepath.Join(home, "projects", pid, "artifacts", "wfi-ow")
	entries, _ := os.ReadDir(parent)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".tmp-") {
			t.Errorf("leftover temp file after overwrite: %s", e.Name())
		}
	}
}

func TestInternalFS_ParallelPut(t *testing.T) {
	home := t.TempDir()
	t.Setenv("NRFLO_HOME", home)

	const pid = "proj-par"
	fs := newInternalFS(pid)
	ctx := context.Background()

	const n = 8
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := "wfi-par/art-" + string(rune('a'+i)) + "__file.txt"
			errs[i] = fs.Put(ctx, key, bytes.NewReader([]byte{byte('a' + i)}))
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d Put() error: %v", i, err)
		}
	}
	for i := range n {
		key := "wfi-par/art-" + string(rune('a'+i)) + "__file.txt"
		if _, err := os.Stat(filepath.Join(home, "projects", pid, "artifacts", key)); err != nil {
			t.Errorf("file %d missing after parallel Put: %v", i, err)
		}
	}
}
