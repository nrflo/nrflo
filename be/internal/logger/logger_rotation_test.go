package logger

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
)

// archivePattern matches rotated log filenames like 20260402_181337.log
var archivePattern = regexp.MustCompile(`^\d{8}_\d{6}\.log$`)

// setupRotationTest initializes the logger in a temp dir with the given rotation
// threshold. Restores all modified package state in t.Cleanup.
func setupRotationTest(t *testing.T, threshold int64) string {
	t.Helper()

	mu.Lock()
	origMaxLogSize := maxLogSize
	mu.Unlock()

	tempDir := t.TempDir()
	lp := filepath.Join(tempDir, "be.log")
	if err := Init(lp); err != nil {
		t.Fatalf("Init(%q): %v", lp, err)
	}

	mu.Lock()
	maxLogSize = threshold
	mu.Unlock()

	t.Cleanup(func() {
		mu.Lock()
		defer mu.Unlock()
		if logFile != nil {
			logFile.Close()
		}
		writer = os.Stderr
		logFile = nil
		logPath = ""
		maxLogSize = origMaxLogSize
	})

	return tempDir
}

// listArchives returns filenames in dir matching the YYYYMMDD_HHMMSS.log pattern.
func listArchives(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir(%q): %v", dir, err)
	}
	var archives []string
	for _, e := range entries {
		if archivePattern.MatchString(e.Name()) {
			archives = append(archives, e.Name())
		}
	}
	return archives
}

// countLinesInDir counts total newlines across all *.log files in dir.
func countLinesInDir(t *testing.T, dir string) int {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir(%q): %v", dir, err)
	}
	total := 0
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".log") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("ReadFile(%q): %v", e.Name(), err)
		}
		total += strings.Count(string(data), "\n")
	}
	return total
}

// TestRotate_TriggersAtThreshold verifies that rotation fires once the file
// exceeds the configured threshold: an archive appears and be.log is recreated.
func TestRotate_TriggersAtThreshold(t *testing.T) {
	// Each line is ~64 bytes; threshold=256 → rotation fires after ~4 writes.
	const threshold int64 = 256
	tempDir := setupRotationTest(t, threshold)
	ctx := WithTrx(context.Background(), "rot00001")

	for i := 0; i < 10; i++ {
		Info(ctx, "rotation-trigger-test", "seq", i)
	}

	archives := listArchives(t, tempDir)
	if len(archives) == 0 {
		t.Fatalf("expected at least one archive in %s after writing beyond threshold, got none", tempDir)
	}
	for _, name := range archives {
		info, err := os.Stat(filepath.Join(tempDir, name))
		if err != nil {
			t.Errorf("archive %q: stat failed: %v", name, err)
			continue
		}
		if info.Size() == 0 {
			t.Errorf("archive %q is empty", name)
		}
	}
	if _, err := os.Stat(filepath.Join(tempDir, "be.log")); err != nil {
		t.Errorf("be.log missing after rotation: %v", err)
	}
}

// TestRotate_NoTriggerBelowThreshold verifies that no archive is created when
// total bytes written stay well below the threshold.
func TestRotate_NoTriggerBelowThreshold(t *testing.T) {
	const threshold int64 = 10 * 1024 * 1024 // keep 10 MB default
	tempDir := setupRotationTest(t, threshold)
	ctx := WithTrx(context.Background(), "norot001")

	for i := 0; i < 5; i++ {
		Info(ctx, "small write below threshold", "i", i)
	}

	archives := listArchives(t, tempDir)
	if len(archives) != 0 {
		t.Errorf("expected no archives below threshold, got: %v", archives)
	}
}

// TestRotate_NewWritesGoToFreshFile verifies that after rotation the archive
// holds pre-rotation content and the fresh be.log receives post-rotation writes.
//
// threshold=400 bytes, each "before" line ≈58 bytes → rotation fires after 7
// writes (7×58=406 ≥ 400). The 3 remaining "before" lines (174 bytes) plus the
// 3 "after" lines (≈192 bytes) total 366 bytes — well under 400, so exactly one
// rotation fires and no archive is overwritten.
func TestRotate_NewWritesGoToFreshFile(t *testing.T) {
	const threshold int64 = 400
	tempDir := setupRotationTest(t, threshold)
	ctx := WithTrx(context.Background(), "newfile1")
	beLogPath := filepath.Join(tempDir, "be.log")

	for i := 0; i < 10; i++ {
		Info(ctx, "BEFORE_ROTATION", "seq", i)
	}

	archives := listArchives(t, tempDir)
	if len(archives) == 0 {
		t.Fatalf("rotation did not fire: no archives found in %s", tempDir)
	}

	for i := 0; i < 3; i++ {
		Info(ctx, "AFTER_ROTATION_UNIQUE", "seq", i)
	}

	current, err := os.ReadFile(beLogPath)
	if err != nil {
		t.Fatalf("ReadFile(be.log): %v", err)
	}
	if !strings.Contains(string(current), "AFTER_ROTATION_UNIQUE") {
		t.Errorf("be.log does not contain AFTER_ROTATION_UNIQUE; content:\n%s", string(current))
	}

	foundBefore := false
	for _, name := range archives {
		data, err := os.ReadFile(filepath.Join(tempDir, name))
		if err != nil {
			t.Fatalf("ReadFile(%s): %v", name, err)
		}
		if strings.Contains(string(data), "BEFORE_ROTATION") {
			foundBefore = true
			break
		}
	}
	if !foundBefore {
		t.Error("no archive contains BEFORE_ROTATION content")
	}
}

// TestRotate_ConcurrentWrites verifies that concurrent logging during rotation
// produces no panics and preserves every written line.
//
// threshold=20 KB, 30 goroutines × 20 writes = 600 lines ≈ 39 KB total.
// The first rotation fires at ~317 lines; the remaining ~283 lines (≈18 KB) stay
// below threshold, so exactly one rotation occurs. Because only one rename
// happens, there is no same-second archive collision, and all 600 lines are
// preserved across the archive and the current be.log.
func TestRotate_ConcurrentWrites(t *testing.T) {
	const threshold int64 = 20 * 1024
	const numGoroutines = 30
	const numLogsEach = 20
	const expected = numGoroutines * numLogsEach

	tempDir := setupRotationTest(t, threshold)
	ctx := WithTrx(context.Background(), "concrot1")

	var wg sync.WaitGroup
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numLogsEach; j++ {
				Info(ctx, "concurrent-rotation", "g", id, "j", j)
			}
		}(i)
	}
	wg.Wait()

	total := countLinesInDir(t, tempDir)
	if total != expected {
		t.Errorf("total log lines across all files = %d, want %d", total, expected)
	}
}
