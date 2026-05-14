package spawner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// geminiChatsDir creates <geminiHome>/.gemini/tmp/<sha256(resolvedWorkDir)>/chats/
// and returns the path. Used by waitForGeminiTranscript tests.
func geminiChatsDir(t *testing.T, geminiHome, resolvedWorkDir string) string {
	t.Helper()
	h := sha256.Sum256([]byte(resolvedWorkDir))
	hash := hex.EncodeToString(h[:])
	dir := filepath.Join(geminiHome, ".gemini", "tmp", hash, "chats")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("geminiChatsDir MkdirAll: %v", err)
	}
	return dir
}

// --- dispatchGeminiJSONL tests ---

// TestDispatchGeminiJSONL_AssistantFirstLine verifies that a single assistant
// line emits one "text" message, one bump, and one setLastMessage call.
func TestDispatchGeminiJSONL_AssistantFirstLine(t *testing.T) {
	t.Parallel()
	sink := &opencodeTestSink{}
	state := &geminiTailState{seenContent: make(map[string]int)}
	line := []byte(`{"id":"t1","role":"assistant","content":"hello"}`)
	dispatchGeminiJSONL("sess-1", line, sink, 200000, state)

	sink.mu.Lock()
	defer sink.mu.Unlock()
	if len(sink.recordedMsgs) != 1 || sink.recordedMsgs[0].category != "text" || sink.recordedMsgs[0].content != "hello" {
		t.Errorf("recordedMsgs = %+v, want one text/'hello'", sink.recordedMsgs)
	}
	if sink.bumpCount != 1 {
		t.Errorf("bumpCount = %d, want 1", sink.bumpCount)
	}
	if len(sink.lastMessages) != 1 || sink.lastMessages[0] != "hello" {
		t.Errorf("lastMessages = %v, want [hello]", sink.lastMessages)
	}
}

// TestDispatchGeminiJSONL_CumulativeDeltaPerID verifies that Gemini's
// cumulative content rewrites are deduplicated: first line emits "hello",
// second line (content="hello world") emits only the delta " world".
func TestDispatchGeminiJSONL_CumulativeDeltaPerID(t *testing.T) {
	t.Parallel()
	sink := &opencodeTestSink{}
	state := &geminiTailState{seenContent: make(map[string]int)}

	line1 := []byte(`{"id":"t1","role":"assistant","content":"hello"}`)
	dispatchGeminiJSONL("sess-1", line1, sink, 200000, state)

	line2 := []byte(`{"id":"t1","role":"assistant","content":"hello world"}`)
	dispatchGeminiJSONL("sess-1", line2, sink, 200000, state)

	sink.mu.Lock()
	defer sink.mu.Unlock()
	if len(sink.recordedMsgs) != 2 {
		t.Fatalf("recordedMsgs = %+v, want 2 messages", sink.recordedMsgs)
	}
	if sink.recordedMsgs[0].content != "hello" {
		t.Errorf("msg[0] = %q, want 'hello'", sink.recordedMsgs[0].content)
	}
	if sink.recordedMsgs[1].content != " world" {
		t.Errorf("msg[1] = %q, want ' world' (delta only)", sink.recordedMsgs[1].content)
	}
}

// TestDispatchGeminiJSONL_SetSentinelFiltered verifies that lines with the
// "$set" advisory field are silently dropped — no messages, bumps, or context
// updates regardless of other fields.
func TestDispatchGeminiJSONL_SetSentinelFiltered(t *testing.T) {
	t.Parallel()
	sink := &opencodeTestSink{}
	state := &geminiTailState{seenContent: make(map[string]int)}
	line := []byte(`{"$set":{"key":"value"},"role":"assistant","content":"ignored"}`)
	dispatchGeminiJSONL("sess-1", line, sink, 200000, state)

	sink.mu.Lock()
	defer sink.mu.Unlock()
	if len(sink.recordedMsgs) != 0 || sink.bumpCount != 0 || len(sink.contextUpdates) != 0 {
		t.Errorf("expected zero activity, got msgs=%d bumps=%d ctx=%d",
			len(sink.recordedMsgs), sink.bumpCount, len(sink.contextUpdates))
	}
}

// TestDispatchGeminiJSONL_ToolCalls verifies that an assistant line with
// tool_calls emits a "tool" message prefixed with [<name>].
func TestDispatchGeminiJSONL_ToolCalls(t *testing.T) {
	t.Parallel()
	sink := &opencodeTestSink{}
	state := &geminiTailState{seenContent: make(map[string]int)}
	line := []byte(`{"id":"t1","role":"assistant","content":"","tool_calls":[{"name":"shell","args":{"cmd":"ls"}}]}`)
	dispatchGeminiJSONL("sess-1", line, sink, 200000, state)

	sink.mu.Lock()
	defer sink.mu.Unlock()
	if len(sink.recordedMsgs) != 1 {
		t.Fatalf("recordedMsgs = %+v, want 1 tool msg", sink.recordedMsgs)
	}
	if sink.recordedMsgs[0].category != "tool" {
		t.Errorf("category = %q, want 'tool'", sink.recordedMsgs[0].category)
	}
	if !strings.HasPrefix(sink.recordedMsgs[0].content, "[shell]") {
		t.Errorf("content = %q, want prefix '[shell]'", sink.recordedMsgs[0].content)
	}
}

// TestDispatchGeminiJSONL_MalformedJSON verifies that invalid JSON lines are
// silently dropped without panicking.
func TestDispatchGeminiJSONL_MalformedJSON(t *testing.T) {
	t.Parallel()
	sink := &opencodeTestSink{}
	state := &geminiTailState{seenContent: make(map[string]int)}
	for _, bad := range [][]byte{
		[]byte(`not json`),
		[]byte(`{`),
	} {
		dispatchGeminiJSONL("sess-1", bad, sink, 200000, state)
	}

	sink.mu.Lock()
	defer sink.mu.Unlock()
	if len(sink.recordedMsgs) != 0 || sink.bumpCount != 0 {
		t.Errorf("expected silence on bad JSON, got msgs=%d bumps=%d",
			len(sink.recordedMsgs), sink.bumpCount)
	}
}

// TestDispatchGeminiJSONL_TokensTotal_UpdatesContext verifies that
// tokens.total → UpdateContextLeft(pct) + BumpLastMessage. With
// total=250000 and maxCtx=1_000_000, pct = 100 - 25 = 75.
func TestDispatchGeminiJSONL_TokensTotal_UpdatesContext(t *testing.T) {
	t.Parallel()
	sink := &opencodeTestSink{}
	state := &geminiTailState{seenContent: make(map[string]int)}
	line := []byte(`{"id":"t1","role":"assistant","content":"","tokens":{"total":250000}}`)
	dispatchGeminiJSONL("sess-1", line, sink, 1000000, state)

	sink.mu.Lock()
	defer sink.mu.Unlock()
	if len(sink.contextUpdates) != 1 || sink.contextUpdates[0] != 75 {
		t.Errorf("contextUpdates = %v, want [75]", sink.contextUpdates)
	}
	if sink.bumpCount != 1 {
		t.Errorf("bumpCount = %d, want 1", sink.bumpCount)
	}
}

// TestDispatchGeminiJSONL_RoleUserSystemSkipped verifies that user and system
// role lines produce no emits — only assistant lines are actionable.
func TestDispatchGeminiJSONL_RoleUserSystemSkipped(t *testing.T) {
	t.Parallel()
	sink := &opencodeTestSink{}
	state := &geminiTailState{seenContent: make(map[string]int)}
	for _, line := range [][]byte{
		[]byte(`{"id":"t1","role":"user","content":"hello"}`),
		[]byte(`{"id":"t2","role":"system","content":"system prompt"}`),
	} {
		dispatchGeminiJSONL("sess-1", line, sink, 200000, state)
	}

	sink.mu.Lock()
	defer sink.mu.Unlock()
	if len(sink.recordedMsgs) != 0 || sink.bumpCount != 0 {
		t.Errorf("expected zero emits, got msgs=%d bumps=%d",
			len(sink.recordedMsgs), sink.bumpCount)
	}
}

// --- waitForGeminiTranscript tests ---

// TestWaitForGeminiTranscript_FindsUUIDMatchingFile verifies the discovery
// loop returns the transcript path whose suffix matches sessionID. A decoy
// file with a different session suffix must not be returned.
func TestWaitForGeminiTranscript_FindsUUIDMatchingFile(t *testing.T) {
	geminiHome := t.TempDir()
	workDir := t.TempDir()
	// macOS t.TempDir returns /var/... which resolves to /private/var/...;
	// compute the hash from the resolved path to match the tailer.
	resolvedWorkDir, err := filepath.EvalSymlinks(workDir)
	if err != nil {
		resolvedWorkDir = workDir
	}

	sessionID := "test-session-abc123"
	chatsDir := geminiChatsDir(t, geminiHome, resolvedWorkDir)

	// Decoy: different session UUID suffix — glob must not match it.
	decoy := filepath.Join(chatsDir, "session-1700000000-decoy-session.jsonl")
	if err := os.WriteFile(decoy, []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("decoy write: %v", err)
	}

	wanted := filepath.Join(chatsDir, "session-1700000000-"+sessionID+".jsonl")
	go func() {
		time.Sleep(100 * time.Millisecond)
		_ = os.WriteFile(wanted, []byte("{}\n"), 0o600)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	t.Cleanup(cancel)
	path, err := waitForGeminiTranscript(ctx, sessionID, geminiHome, resolvedWorkDir, 2*time.Second)
	if err != nil {
		t.Fatalf("waitForGeminiTranscript: %v", err)
	}
	if !strings.HasSuffix(path, "-"+sessionID+".jsonl") {
		t.Errorf("path = %q, want suffix '-%s.jsonl'; decoy must not match", path, sessionID)
	}
}

// TestWaitForGeminiTranscript_DeadlineExceeded verifies that when no matching
// file appears within the deadline, a non-nil error is returned.
func TestWaitForGeminiTranscript_DeadlineExceeded(t *testing.T) {
	geminiHome := t.TempDir()
	// Use a stable path we control — no symlink ambiguity needed for the hash.
	resolvedWorkDir := "/private/var/folders/x/test-gemini-deadline"

	// Create the chats dir but put no matching file.
	_ = geminiChatsDir(t, geminiHome, resolvedWorkDir)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	t.Cleanup(cancel)
	_, err := waitForGeminiTranscript(ctx, "sess-deadline", geminiHome, resolvedWorkDir, 200*time.Millisecond)
	if err == nil {
		t.Error("expected non-nil error on deadline exceeded, got nil")
	}
}

// TestWaitForGeminiTranscript_CtxCanceled verifies that a pre-canceled context
// causes the function to return ctx.Err() immediately.
func TestWaitForGeminiTranscript_CtxCanceled(t *testing.T) {
	geminiHome := t.TempDir()
	resolvedWorkDir := "/private/var/folders/x/test-gemini-cancel"

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before calling — ctx.Done() is already closed

	_, err := waitForGeminiTranscript(ctx, "sess-cancel", geminiHome, resolvedWorkDir, 5*time.Second)
	if err == nil {
		t.Fatal("expected non-nil error from canceled ctx, got nil")
	}
	if err != ctx.Err() {
		t.Errorf("err = %v, want ctx.Err() = %v", err, ctx.Err())
	}
}
