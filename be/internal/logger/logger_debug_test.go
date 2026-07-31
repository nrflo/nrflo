package logger

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// TestDebug_OutputFormat verifies Debug renders the same shape as Info/Warn:
// level, message, trx, and key=value args (mirrors TestInfo_OutputFormat /
// TestWarn_OutputFormat).
func TestDebug_OutputFormat(t *testing.T) {
	var buf bytes.Buffer
	mu.Lock()
	writer = &buf
	mu.Unlock()

	ctx := WithTrx(context.Background(), "dbg12345")

	Debug(ctx, "record_event: PreToolUse received", "session_id", "sess-1", "tool", "Bash")

	output := buf.String()

	if !strings.Contains(output, "DEBUG") {
		t.Errorf("output missing DEBUG level: %s", output)
	}
	if !strings.Contains(output, "[dbg12345]") {
		t.Errorf("output missing trx [dbg12345]: %s", output)
	}
	if !strings.Contains(output, "record_event: PreToolUse received") {
		t.Errorf("output missing message: %s", output)
	}
	if !strings.Contains(output, "session_id=sess-1") {
		t.Errorf("output missing session_id=sess-1: %s", output)
	}
	if !strings.Contains(output, "tool=Bash") {
		t.Errorf("output missing tool=Bash: %s", output)
	}
}
