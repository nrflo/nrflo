package apirun

import (
	"strings"
	"testing"
)

// TestFormatToolResult_Success verifies the "[<name>] -> <out>" form for non-error results.
func TestFormatToolResult_Success(t *testing.T) {
	got := formatToolResult("Bash", "file.txt\nREADME.md", false)
	want := "[Bash] → file.txt\nREADME.md"
	if got != want {
		t.Errorf("formatToolResult success = %q, want %q", got, want)
	}
}

// TestFormatToolResult_Error verifies the "<name>: <out>" form for error results.
func TestFormatToolResult_Error(t *testing.T) {
	got := formatToolResult("Bash", "permission denied", true)
	want := "Bash: permission denied"
	if got != want {
		t.Errorf("formatToolResult error = %q, want %q", got, want)
	}
}

// TestFormatToolResult_SuccessTruncatesAt2048 verifies that successful output
// longer than 2048 bytes is truncated.
func TestFormatToolResult_SuccessTruncatesAt2048(t *testing.T) {
	longOut := strings.Repeat("a", 3000)
	got := formatToolResult("Read", longOut, false)
	if !strings.HasPrefix(got, "[Read] → ") {
		t.Errorf("formatToolResult = %q, want [Read] → prefix", got)
	}
	// output portion should be 2048 chars exactly
	wantOut := strings.Repeat("a", 2048)
	if !strings.HasSuffix(got, wantOut) {
		t.Errorf("formatToolResult truncated output len = %d, want %d", len(got)-len("[Read] → "), 2048)
	}
}

// TestFormatToolResult_ErrorNoTruncation verifies error output is not truncated.
func TestFormatToolResult_ErrorNoTruncation(t *testing.T) {
	longOut := strings.Repeat("e", 4000)
	got := formatToolResult("Write", longOut, true)
	want := "Write: " + longOut
	if got != want {
		t.Errorf("formatToolResult error truncated unexpectedly: len=%d, want %d", len(got), len(want))
	}
}

// TestFormatToolResult_EmptyOutput verifies zero-length output is formatted correctly.
func TestFormatToolResult_EmptyOutput(t *testing.T) {
	cases := []struct {
		isErr bool
		want  string
	}{
		{false, "[Tool] → "},
		{true, "Tool: "},
	}
	for _, tc := range cases {
		got := formatToolResult("Tool", "", tc.isErr)
		if got != tc.want {
			t.Errorf("formatToolResult(empty, isErr=%v) = %q, want %q", tc.isErr, got, tc.want)
		}
	}
}
