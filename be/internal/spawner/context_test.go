package spawner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

const contextFilePath = "/tmp/nrworkflow/usable_context.json"

// writeContextFile writes content to the context file and returns a cleanup function.
func writeContextFile(t *testing.T, content []byte) func() {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(contextFilePath), 0o755); err != nil {
		t.Fatalf("failed to create context file dir: %v", err)
	}

	// Preserve any existing file.
	existing, readErr := os.ReadFile(contextFilePath)

	if err := os.WriteFile(contextFilePath, content, 0o644); err != nil {
		t.Fatalf("failed to write context file: %v", err)
	}

	return func() {
		if readErr == nil {
			_ = os.WriteFile(contextFilePath, existing, 0o644)
		} else {
			_ = os.Remove(contextFilePath)
		}
	}
}

// removeContextFile removes the context file and returns a cleanup function.
func removeContextFile(t *testing.T) func() {
	t.Helper()

	existing, readErr := os.ReadFile(contextFilePath)
	_ = os.Remove(contextFilePath)

	return func() {
		if readErr == nil {
			_ = os.WriteFile(contextFilePath, existing, 0o644)
		}
	}
}

// TestReadContextFile_ValidJSON tests that readContextFile parses valid JSON correctly.
func TestReadContextFile_ValidJSON(t *testing.T) {
	pct := 42.5
	entries := map[string]contextFileEntry{
		"session-abc": {PctUsed: &pct},
	}
	data, err := json.Marshal(entries)
	if err != nil {
		t.Fatalf("failed to marshal test data: %v", err)
	}
	cleanup := writeContextFile(t, data)
	defer cleanup()

	result := readContextFile()

	if result == nil {
		t.Fatalf("expected non-nil result, got nil")
	}
	entry, ok := result["session-abc"]
	if !ok {
		t.Fatalf("expected key 'session-abc' in result")
	}
	if entry.PctUsed == nil {
		t.Errorf("expected PctUsed to be non-nil")
	} else if *entry.PctUsed != 42.5 {
		t.Errorf("readContextFile() PctUsed = %v, want 42.5", *entry.PctUsed)
	}
}

// TestReadContextFile_MultipleEntries tests multiple session entries.
func TestReadContextFile_MultipleEntries(t *testing.T) {
	pct1 := 10.0
	pct2 := 80.0
	entries := map[string]contextFileEntry{
		"session-1": {PctUsed: &pct1},
		"session-2": {PctUsed: &pct2},
	}
	data, err := json.Marshal(entries)
	if err != nil {
		t.Fatalf("failed to marshal test data: %v", err)
	}
	cleanup := writeContextFile(t, data)
	defer cleanup()

	result := readContextFile()

	if result == nil {
		t.Fatalf("expected non-nil result, got nil")
	}
	if len(result) != 2 {
		t.Errorf("readContextFile() len = %d, want 2", len(result))
	}
}

// TestReadContextFile_NullPctUsed tests that an entry with null pct_used is parsed.
func TestReadContextFile_NullPctUsed(t *testing.T) {
	data := []byte(`{"session-xyz": {"pct_used": null}}`)
	cleanup := writeContextFile(t, data)
	defer cleanup()

	result := readContextFile()

	if result == nil {
		t.Fatalf("expected non-nil result, got nil")
	}
	entry, ok := result["session-xyz"]
	if !ok {
		t.Fatalf("expected key 'session-xyz' in result")
	}
	if entry.PctUsed != nil {
		t.Errorf("expected PctUsed to be nil for null JSON value")
	}
}

// TestReadContextFile_FileNotFound tests that a missing file returns nil.
func TestReadContextFile_FileNotFound(t *testing.T) {
	cleanup := removeContextFile(t)
	defer cleanup()

	result := readContextFile()

	if result != nil {
		t.Errorf("readContextFile() = %v, want nil when file not found", result)
	}
}

// TestReadContextFile_MalformedJSON tests that malformed JSON returns nil.
func TestReadContextFile_MalformedJSON(t *testing.T) {
	cleanup := writeContextFile(t, []byte(`{not valid json`))
	defer cleanup()

	result := readContextFile()

	if result != nil {
		t.Errorf("readContextFile() = %v, want nil for malformed JSON", result)
	}
}

// TestReadContextFile_EmptyObject tests that an empty JSON object returns empty map.
func TestReadContextFile_EmptyObject(t *testing.T) {
	cleanup := writeContextFile(t, []byte(`{}`))
	defer cleanup()

	result := readContextFile()

	if result == nil {
		t.Fatalf("expected non-nil result for empty JSON object")
	}
	if len(result) != 0 {
		t.Errorf("readContextFile() len = %d, want 0", len(result))
	}
}

// TestReadContextFile_EmptyFile tests that an empty file returns nil.
func TestReadContextFile_EmptyFile(t *testing.T) {
	cleanup := writeContextFile(t, []byte{})
	defer cleanup()

	result := readContextFile()

	if result != nil {
		t.Errorf("readContextFile() = %v, want nil for empty file", result)
	}
}

// TestUpdateContextLeft_UpdatesWhenDiffers tests that contextLeft is updated
// and dirty flag is set when the value differs.
func TestUpdateContextLeft_UpdatesWhenDiffers(t *testing.T) {
	pct := 40.0 // 40% used → 60% remaining
	contextData := map[string]contextFileEntry{
		"sess-1": {PctUsed: &pct},
	}
	proc := &processInfo{
		sessionID:   "sess-1",
		contextLeft: 50,
	}

	updateContextLeft(proc, contextData)

	if proc.contextLeft != 60 {
		t.Errorf("contextLeft = %d, want 60", proc.contextLeft)
	}
	if !proc.contextLeftDirty {
		t.Errorf("expected contextLeftDirty to be true after change")
	}
}

// TestUpdateContextLeft_NoChangeWhenSame tests that dirty flag is not set
// when the value is unchanged.
func TestUpdateContextLeft_NoChangeWhenSame(t *testing.T) {
	pct := 40.0 // 60% remaining
	contextData := map[string]contextFileEntry{
		"sess-1": {PctUsed: &pct},
	}
	proc := &processInfo{
		sessionID:        "sess-1",
		contextLeft:      60, // already matches
		contextLeftDirty: false,
	}

	updateContextLeft(proc, contextData)

	if proc.contextLeftDirty {
		t.Errorf("expected contextLeftDirty to remain false when value unchanged")
	}
	if proc.contextLeft != 60 {
		t.Errorf("contextLeft = %d, want 60", proc.contextLeft)
	}
}

// TestUpdateContextLeft_NilContextData tests that nil contextData is a no-op.
func TestUpdateContextLeft_NilContextData(t *testing.T) {
	proc := &processInfo{
		sessionID:   "sess-1",
		contextLeft: 75,
	}

	updateContextLeft(proc, nil)

	if proc.contextLeft != 75 {
		t.Errorf("contextLeft = %d, want 75", proc.contextLeft)
	}
	if proc.contextLeftDirty {
		t.Errorf("expected contextLeftDirty to remain false")
	}
}

// TestUpdateContextLeft_SessionNotInContextData tests that missing session is a no-op.
func TestUpdateContextLeft_SessionNotInContextData(t *testing.T) {
	pct := 20.0
	contextData := map[string]contextFileEntry{
		"other-session": {PctUsed: &pct},
	}
	proc := &processInfo{
		sessionID:   "sess-1",
		contextLeft: 75,
	}

	updateContextLeft(proc, contextData)

	if proc.contextLeft != 75 {
		t.Errorf("contextLeft = %d, want 75", proc.contextLeft)
	}
	if proc.contextLeftDirty {
		t.Errorf("expected contextLeftDirty to remain false")
	}
}

// TestUpdateContextLeft_NilPctUsed tests that nil PctUsed is a no-op.
func TestUpdateContextLeft_NilPctUsed(t *testing.T) {
	contextData := map[string]contextFileEntry{
		"sess-1": {PctUsed: nil},
	}
	proc := &processInfo{
		sessionID:   "sess-1",
		contextLeft: 75,
	}

	updateContextLeft(proc, contextData)

	if proc.contextLeft != 75 {
		t.Errorf("contextLeft = %d, want 75", proc.contextLeft)
	}
	if proc.contextLeftDirty {
		t.Errorf("expected contextLeftDirty to remain false")
	}
}

// TestUpdateContextLeft_ZeroPctUsed tests 0% used → 100% remaining.
func TestUpdateContextLeft_ZeroPctUsed(t *testing.T) {
	pct := 0.0
	contextData := map[string]contextFileEntry{
		"sess-1": {PctUsed: &pct},
	}
	proc := &processInfo{
		sessionID:   "sess-1",
		contextLeft: 50,
	}

	updateContextLeft(proc, contextData)

	if proc.contextLeft != 100 {
		t.Errorf("contextLeft = %d, want 100", proc.contextLeft)
	}
	if !proc.contextLeftDirty {
		t.Errorf("expected contextLeftDirty to be true")
	}
}

// TestUpdateContextLeft_FullyUsed tests 100% used → 0% remaining.
func TestUpdateContextLeft_FullyUsed(t *testing.T) {
	pct := 100.0
	contextData := map[string]contextFileEntry{
		"sess-1": {PctUsed: &pct},
	}
	proc := &processInfo{
		sessionID:   "sess-1",
		contextLeft: 50,
	}

	updateContextLeft(proc, contextData)

	if proc.contextLeft != 0 {
		t.Errorf("contextLeft = %d, want 0", proc.contextLeft)
	}
	if !proc.contextLeftDirty {
		t.Errorf("expected contextLeftDirty to be true")
	}
}
