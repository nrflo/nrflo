package spawner

import (
	"os"
	"path/filepath"
	"testing"
)

// TestReadNewLines_RespectsOffset verifies incremental reads emit only complete
// lines and carry partial trailing lines to the next call.
func TestReadNewLines_RespectsOffset(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "rollout.jsonl")

	// First write: two full lines + one partial.
	if err := os.WriteFile(path, []byte("line1\nline2\nparti"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	var got []string
	off := readNewLines(path, 0, func(line []byte) { got = append(got, string(line)) })
	if len(got) != 2 || got[0] != "line1" || got[1] != "line2" {
		t.Errorf("first pass got = %v, want [line1 line2]", got)
	}
	// off must point at end of last complete newline (after "line2\n" = 12 bytes).
	if off != 12 {
		t.Errorf("offset = %d, want 12", off)
	}

	// Now append the rest of the partial line + a new line.
	if err := os.WriteFile(path, []byte("line1\nline2\npartial\nline4\n"), 0o600); err != nil {
		t.Fatalf("write2: %v", err)
	}
	got = nil
	off2 := readNewLines(path, off, func(line []byte) { got = append(got, string(line)) })
	if len(got) != 2 || got[0] != "partial" || got[1] != "line4" {
		t.Errorf("second pass got = %v, want [partial line4]", got)
	}
	if off2 != 26 {
		t.Errorf("offset2 = %d, want 26", off2)
	}
}
