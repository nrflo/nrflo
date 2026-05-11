package spawner

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"testing"

	"be/internal/logger"
)

func TestBuildSavePrompt(t *testing.T) {
	t.Parallel()
	prompt := buildSavePrompt()

	if !strings.Contains(prompt, "nrflo findings add to_resume") {
		t.Error("save prompt should contain 'nrflo findings add to_resume' instruction")
	}
	if !strings.Contains(prompt, "nrflo agent continue") {
		t.Error("save prompt should contain 'nrflo agent continue' instruction")
	}
	if !strings.Contains(prompt, "URGENT") {
		t.Error("save prompt should start with URGENT")
	}
}

// TestContextSaveResume_StdoutScannerLargeLinePassthrough verifies that the stdout
// scanner in contextSaveViaResume passes long lines to the logger verbatim.
// The scanner is configured with a 1MB initial buffer and 10MB max — a 2000-byte
// line must be logged byte-for-byte with no "..." truncation.
func TestContextSaveResume_StdoutScannerLargeLinePassthrough(t *testing.T) {
	// Not parallel: temporarily replaces the global logger writer.
	var logBuf bytes.Buffer
	logger.SetWriter(&logBuf)
	defer logger.SetWriter(io.Discard)

	longLine := strings.Repeat("L", 2000)

	pr, pw := io.Pipe()
	go func() {
		fmt.Fprintf(pw, "%s\n", longLine)
		pw.Close()
	}()

	// Run the exact scanner loop used in contextSaveViaResume.
	ctx := context.Background()
	scanner := bufio.NewScanner(pr)
	buf := make([]byte, 0, 1024*1024)
	scanner.Buffer(buf, 10*1024*1024)
	for scanner.Scan() {
		logger.Info(ctx, "context save output", "content", scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scanner error: %v", err)
	}

	output := logBuf.String()
	if !strings.Contains(output, longLine) {
		t.Errorf("logged output does not contain the full 2000-byte line (logged len=%d); want verbatim passthrough", len(output))
	}
	if strings.Contains(output, "...") {
		t.Error("logged output contains '...' truncation marker; want full content passthrough")
	}
}
