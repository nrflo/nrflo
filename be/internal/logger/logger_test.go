package logger

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNewTrx(t *testing.T) {
	tests := []struct {
		name string
	}{
		{"generates 8-char hex string"},
		{"second call"},
		{"third call"},
	}

	seen := make(map[string]bool)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			trx := NewTrx()
			if len(trx) != 8 {
				t.Errorf("NewTrx() length = %d, want 8", len(trx))
			}
			// Verify it's valid hex
			for _, ch := range trx {
				if !((ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f')) {
					t.Errorf("NewTrx() = %q contains non-hex char %c", trx, ch)
				}
			}
			// Verify uniqueness
			if seen[trx] {
				t.Errorf("NewTrx() returned duplicate value %q", trx)
			}
			seen[trx] = true
		})
	}
}

func TestWithTrx_TrxFromContext_RoundTrip(t *testing.T) {
	tests := []struct {
		name string
		trx  string
		want string
	}{
		{"valid trx", "abc12345", "abc12345"},
		{"empty trx", "", "-"},
		{"short trx", "xyz", "xyz"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			ctx = WithTrx(ctx, tt.trx)
			got := TrxFromContext(ctx)
			if got != tt.want {
				t.Errorf("TrxFromContext() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTrxFromContext_EmptyContext(t *testing.T) {
	ctx := context.Background()
	got := TrxFromContext(ctx)
	if got != "-" {
		t.Errorf("TrxFromContext(empty context) = %q, want '-'", got)
	}
}

func TestInfo_OutputFormat(t *testing.T) {
	var buf bytes.Buffer
	mu.Lock()
	writer = &buf
	mu.Unlock()

	ctx := context.Background()
	ctx = WithTrx(ctx, "test1234")

	Info(ctx, "server started", "port", 8080, "db", "test.db")

	output := buf.String()

	// Check timestamp format (YYYY-MM-DD HH:MM:SS)
	if !strings.Contains(output, time.Now().Format("2006-01-02")) {
		t.Errorf("output missing date: %s", output)
	}

	// Check level
	if !strings.Contains(output, "INFO") {
		t.Errorf("output missing INFO level: %s", output)
	}

	// Check trx
	if !strings.Contains(output, "[test1234]") {
		t.Errorf("output missing trx [test1234]: %s", output)
	}

	// Check message
	if !strings.Contains(output, "server started") {
		t.Errorf("output missing message: %s", output)
	}

	// Check key-value pairs
	if !strings.Contains(output, "port=8080") {
		t.Errorf("output missing port=8080: %s", output)
	}
	if !strings.Contains(output, "db=test.db") {
		t.Errorf("output missing db=test.db: %s", output)
	}

	// Check newline
	if !strings.HasSuffix(output, "\n") {
		t.Errorf("output missing newline: %s", output)
	}
}

func TestWarn_OutputFormat(t *testing.T) {
	var buf bytes.Buffer
	mu.Lock()
	writer = &buf
	mu.Unlock()

	ctx := WithTrx(context.Background(), "warn5678")

	Warn(ctx, "high memory usage", "used_mb", 4096)

	output := buf.String()

	if !strings.Contains(output, "WARN") {
		t.Errorf("output missing WARN level: %s", output)
	}
	if !strings.Contains(output, "[warn5678]") {
		t.Errorf("output missing trx: %s", output)
	}
	if !strings.Contains(output, "high memory usage") {
		t.Errorf("output missing message: %s", output)
	}
	if !strings.Contains(output, "used_mb=4096") {
		t.Errorf("output missing key-value: %s", output)
	}
}

func TestError_OutputFormat(t *testing.T) {
	var buf bytes.Buffer
	mu.Lock()
	writer = &buf
	mu.Unlock()

	ctx := WithTrx(context.Background(), "err9abc")

	Error(ctx, "database connection failed", "error", "timeout")

	output := buf.String()

	if !strings.Contains(output, "ERROR") {
		t.Errorf("output missing ERROR level: %s", output)
	}
	if !strings.Contains(output, "[err9abc]") {
		t.Errorf("output missing trx: %s", output)
	}
	if !strings.Contains(output, "database connection failed") {
		t.Errorf("output missing message: %s", output)
	}
	if !strings.Contains(output, "error=timeout") {
		t.Errorf("output missing key-value: %s", output)
	}
}

func TestLog_NoTrxInContext(t *testing.T) {
	var buf bytes.Buffer
	mu.Lock()
	writer = &buf
	mu.Unlock()

	ctx := context.Background()

	Info(ctx, "no trx here")

	output := buf.String()

	if !strings.Contains(output, "[-]") {
		t.Errorf("output missing [-] for empty trx: %s", output)
	}
}

func TestLog_OddNumberOfArgs(t *testing.T) {
	var buf bytes.Buffer
	mu.Lock()
	writer = &buf
	mu.Unlock()

	ctx := WithTrx(context.Background(), "odd12345")

	// Odd number of args - last key should still appear
	Info(ctx, "test message", "key1", "val1", "key2")

	output := buf.String()

	// Should contain the message and valid key-value pairs
	if !strings.Contains(output, "test message") {
		t.Errorf("output missing message: %s", output)
	}
	if !strings.Contains(output, "key1=val1") {
		t.Errorf("output missing key1=val1: %s", output)
	}
	// key2 should be ignored (no value)
}

func TestLog_NoKeyValuePairs(t *testing.T) {
	var buf bytes.Buffer
	mu.Lock()
	writer = &buf
	mu.Unlock()

	ctx := WithTrx(context.Background(), "simple01")

	Info(ctx, "just a message")

	output := buf.String()

	if !strings.Contains(output, "just a message") {
		t.Errorf("output missing message: %s", output)
	}
	if !strings.Contains(output, "[simple01]") {
		t.Errorf("output missing trx: %s", output)
	}
}

func TestLog_MultilineMessage(t *testing.T) {
	var buf bytes.Buffer
	mu.Lock()
	writer = &buf
	mu.Unlock()

	ctx := WithTrx(context.Background(), "multi123")

	Info(ctx, "line one\nline two\nline three")

	output := buf.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")

	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d: %q", len(lines), output)
	}

	for i, line := range lines {
		if !strings.Contains(line, "INFO") {
			t.Errorf("line %d missing INFO: %s", i, line)
		}
		if !strings.Contains(line, "[multi123]") {
			t.Errorf("line %d missing trx: %s", i, line)
		}
	}

	if !strings.Contains(lines[0], "line one") {
		t.Errorf("line 0 missing content: %s", lines[0])
	}
	if !strings.Contains(lines[1], "line two") {
		t.Errorf("line 1 missing content: %s", lines[1])
	}
	if !strings.Contains(lines[2], "line three") {
		t.Errorf("line 2 missing content: %s", lines[2])
	}
}

func TestLog_MultilineMessageWithArgs(t *testing.T) {
	var buf bytes.Buffer
	mu.Lock()
	writer = &buf
	mu.Unlock()

	ctx := WithTrx(context.Background(), "mlarg123")

	Info(ctx, "first\nsecond", "key", "val")

	output := buf.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")

	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d: %q", len(lines), output)
	}

	for i, line := range lines {
		if !strings.Contains(line, "key=val") {
			t.Errorf("line %d missing key=val: %s", i, line)
		}
		if !strings.Contains(line, "[mlarg123]") {
			t.Errorf("line %d missing trx: %s", i, line)
		}
	}
}

func TestInit_CreatesDirectory(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "logs", "subdir", "test.log")

	err := Init(logPath)
	if err != nil {
		t.Fatalf("Init(%q) returned error: %v", logPath, err)
	}

	// Verify directory was created
	dir := filepath.Dir(logPath)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Errorf("Init did not create directory %s", dir)
	}

	// Verify file was created
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		t.Errorf("Init did not create log file %s", logPath)
	}
}

func TestInit_CreatesWritableFile(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "test.log")

	err := Init(logPath)
	if err != nil {
		t.Fatalf("Init(%q) returned error: %v", logPath, err)
	}

	// Write a log message
	ctx := WithTrx(context.Background(), "init1234")
	Info(ctx, "test write after init")

	// Read the file
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	output := string(content)
	if !strings.Contains(output, "test write after init") {
		t.Errorf("log file missing message: %s", output)
	}
	if !strings.Contains(output, "[init1234]") {
		t.Errorf("log file missing trx: %s", output)
	}
}

func TestInit_InvalidPath(t *testing.T) {
	// Try to create a log file in a path that can't be created (requires root)
	err := Init("/root/noaccess/test.log")
	if err == nil {
		t.Error("Init with invalid path should return error")
	}
}

func TestInit_MultiWriter(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "multi.log")

	// Capture stderr
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	defer func() {
		os.Stderr = oldStderr
	}()

	err := Init(logPath)
	if err != nil {
		t.Fatalf("Init(%q) returned error: %v", logPath, err)
	}

	ctx := WithTrx(context.Background(), "multi567")
	Info(ctx, "multi-writer test")

	// Close write end and read from pipe
	w.Close()
	var stderrBuf bytes.Buffer
	io.Copy(&stderrBuf, r)

	// Check file content
	fileContent, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	fileOutput := string(fileContent)
	stderrOutput := stderrBuf.String()

	// Both should contain the message
	if !strings.Contains(fileOutput, "multi-writer test") {
		t.Errorf("log file missing message: %s", fileOutput)
	}
	if !strings.Contains(stderrOutput, "multi-writer test") {
		t.Errorf("stderr missing message: %s", stderrOutput)
	}
}

func TestConcurrentLogging(t *testing.T) {
	var buf bytes.Buffer
	mu.Lock()
	writer = &buf
	mu.Unlock()

	ctx := WithTrx(context.Background(), "conc1234")

	const numGoroutines = 100
	const numLogsPerGoroutine = 10

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numLogsPerGoroutine; j++ {
				Info(ctx, "concurrent log", "goroutine", id, "iteration", j)
			}
		}(i)
	}

	wg.Wait()

	output := buf.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")

	expectedLines := numGoroutines * numLogsPerGoroutine
	if len(lines) != expectedLines {
		t.Errorf("expected %d log lines, got %d", expectedLines, len(lines))
	}

	// Verify each line is well-formed
	for i, line := range lines {
		if !strings.Contains(line, "INFO") {
			t.Errorf("line %d missing INFO: %s", i, line)
		}
		if !strings.Contains(line, "[conc1234]") {
			t.Errorf("line %d missing trx: %s", i, line)
		}
		if !strings.Contains(line, "concurrent log") {
			t.Errorf("line %d missing message: %s", i, line)
		}
	}
}

func TestConcurrentLogging_DifferentContexts(t *testing.T) {
	var buf bytes.Buffer
	mu.Lock()
	writer = &buf
	mu.Unlock()

	const numGoroutines = 50

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			trx := NewTrx()
			ctx := WithTrx(context.Background(), trx)
			Info(ctx, "goroutine log", "id", id)
		}(i)
	}

	wg.Wait()

	output := buf.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")

	if len(lines) != numGoroutines {
		t.Errorf("expected %d log lines, got %d", numGoroutines, len(lines))
	}

	// Extract all trx IDs
	trxSet := make(map[string]bool)
	for _, line := range lines {
		// Extract trx from [trx] pattern
		start := strings.Index(line, "[")
		end := strings.Index(line, "]")
		if start == -1 || end == -1 {
			t.Errorf("line missing trx brackets: %s", line)
			continue
		}
		trx := line[start+1 : end]
		if len(trx) != 8 {
			t.Errorf("invalid trx length in line: %s", line)
		}
		trxSet[trx] = true
	}

	// Most trx IDs should be unique (small chance of collision with random generation)
	if len(trxSet) < numGoroutines-5 {
		t.Errorf("expected mostly unique trx IDs, got %d unique out of %d", len(trxSet), numGoroutines)
	}
}

func TestInit_Append(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "append.log")

	// First init and write
	err := Init(logPath)
	if err != nil {
		t.Fatalf("First Init(%q) returned error: %v", logPath, err)
	}

	ctx := WithTrx(context.Background(), "first123")
	Info(ctx, "first message")

	// Second init (simulating server restart) and write
	err = Init(logPath)
	if err != nil {
		t.Fatalf("Second Init(%q) returned error: %v", logPath, err)
	}

	ctx = WithTrx(context.Background(), "second45")
	Info(ctx, "second message")

	// Read the file
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	output := string(content)

	// Both messages should be present (append mode)
	if !strings.Contains(output, "first message") {
		t.Errorf("log file missing first message: %s", output)
	}
	if !strings.Contains(output, "second message") {
		t.Errorf("log file missing second message: %s", output)
	}

	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) < 2 {
		t.Errorf("expected at least 2 lines in appended log file, got %d", len(lines))
	}
}
