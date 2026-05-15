package spawner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// codexJSONLTestSink mirrors opencodeTestSink: records Sink calls for asserts.
type codexJSONLTestSink struct {
	mu             sync.Mutex
	msgs           []recordedMsg // reuses opencodeTestSink's struct
	bumps          int
	contextUpdates []int
	turnCompletes  int
	setLastMsgs    []string
}

func (s *codexJSONLTestSink) RecordHookMessage(sessionID, content, category, payload string) (string, string, string, error) {
	s.mu.Lock()
	s.msgs = append(s.msgs, recordedMsg{content, category})
	s.mu.Unlock()
	return "proj", "t1", "feature", nil
}
func (s *codexJSONLTestSink) UpdateContextLeft(sessionID string, pct int) (string, string, string, error) {
	s.mu.Lock()
	s.contextUpdates = append(s.contextUpdates, pct)
	s.mu.Unlock()
	return "proj", "t1", "feature", nil
}
func (s *codexJSONLTestSink) BumpLastMessage(sessionID string) {
	s.mu.Lock()
	s.bumps++
	s.mu.Unlock()
}
func (s *codexJSONLTestSink) SetLastMessage(sessionID, content string) {
	s.mu.Lock()
	s.setLastMsgs = append(s.setLastMsgs, content)
	s.mu.Unlock()
}
func (s *codexJSONLTestSink) OnTurnComplete(sessionID string) {
	s.mu.Lock()
	s.turnCompletes++
	s.mu.Unlock()
}
func (s *codexJSONLTestSink) BroadcastMessagesUpdated(projectID, ticketID, workflow, sessionID string) {
}
func (s *codexJSONLTestSink) RecordError(projectID, errType, sessionID, msg string) {}

// TestDispatchCodexJSONL_AgentMessage verifies event_msg/agent_message records
// land as agent_messages text rows + bump heartbeat + populate the periodic
// status-log preview.
func TestDispatchCodexJSONL_AgentMessage(t *testing.T) {
	t.Parallel()
	sink := &codexJSONLTestSink{}
	line := []byte(`{"type":"event_msg","payload":{"type":"agent_message","message":"hello world"}}`)
	dispatchCodexJSONL("sess-1", line, sink)

	sink.mu.Lock()
	defer sink.mu.Unlock()
	if len(sink.msgs) != 1 || sink.msgs[0].category != "text" || sink.msgs[0].content != "hello world" {
		t.Errorf("msgs = %+v, want one text/'hello world'", sink.msgs)
	}
	if sink.bumps != 1 {
		t.Errorf("bumps = %d, want 1", sink.bumps)
	}
	if len(sink.setLastMsgs) != 1 || sink.setLastMsgs[0] != "hello world" {
		t.Errorf("setLastMsgs = %v, want [hello world]", sink.setLastMsgs)
	}
}

// TestDispatchCodexJSONL_TokenCount_WithInfo verifies token_count records with
// last_token_usage.input_tokens populated call UpdateContextLeft with the
// correct percentage and bump the heartbeat.
func TestDispatchCodexJSONL_TokenCount_WithInfo(t *testing.T) {
	t.Parallel()
	sink := &codexJSONLTestSink{}
	// 50% used → 50% left.
	line := []byte(`{"type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":100000},"model_context_window":200000}}}`)
	dispatchCodexJSONL("sess-1", line, sink)

	sink.mu.Lock()
	defer sink.mu.Unlock()
	if len(sink.contextUpdates) != 1 || sink.contextUpdates[0] != 50 {
		t.Errorf("contextUpdates = %v, want [50]", sink.contextUpdates)
	}
	if sink.bumps != 1 {
		t.Errorf("bumps = %d, want 1", sink.bumps)
	}
}

// TestDispatchCodexJSONL_TokenCount_NullInfo verifies the codex 0.130+ shape
// where payload.info is null (only rate_limits present): no context update,
// no bump (we can't extract anything actionable).
func TestDispatchCodexJSONL_TokenCount_NullInfo(t *testing.T) {
	t.Parallel()
	sink := &codexJSONLTestSink{}
	line := []byte(`{"type":"event_msg","payload":{"type":"token_count","info":null,"rate_limits":{"primary":{"used_percent":9.0}}}}`)
	dispatchCodexJSONL("sess-1", line, sink)

	sink.mu.Lock()
	defer sink.mu.Unlock()
	if len(sink.contextUpdates) != 0 {
		t.Errorf("contextUpdates = %v, want []", sink.contextUpdates)
	}
	if sink.bumps != 0 {
		t.Errorf("bumps = %d, want 0 (null info should be silently ignored)", sink.bumps)
	}
}

// TestDispatchCodexJSONL_FunctionCall verifies response_item/function_call
// lands as a "tool" agent_message with `[<name>] <args>` formatting. The
// "tool" category matches what Claude/opencode emit so UI filters and scenario
// assertions work uniformly across backends.
func TestDispatchCodexJSONL_FunctionCall(t *testing.T) {
	t.Parallel()
	sink := &codexJSONLTestSink{}
	line := []byte(`{"type":"response_item","payload":{"type":"function_call","name":"exec_command","arguments":"{\"cmd\":\"ls -la\"}","call_id":"c1"}}`)
	dispatchCodexJSONL("sess-1", line, sink)

	sink.mu.Lock()
	defer sink.mu.Unlock()
	if len(sink.msgs) != 1 || sink.msgs[0].category != "tool" {
		t.Errorf("msgs = %+v, want one tool", sink.msgs)
	}
	if !strings.HasPrefix(sink.msgs[0].content, "[exec_command]") {
		t.Errorf("content = %q, want prefix [exec_command]", sink.msgs[0].content)
	}
	if sink.bumps != 1 {
		t.Errorf("bumps = %d, want 1", sink.bumps)
	}
}

// TestDispatchCodexJSONL_FunctionCallOutputDropped verifies
// response_item/function_call_output records are silently dropped: tool
// results are not surfaced to the UI, matching Claude/opencode behavior
// (handleClaudeToolResult in output.go also drops all tool_results except
// Task/Agent subagent results).
func TestDispatchCodexJSONL_FunctionCallOutputDropped(t *testing.T) {
	t.Parallel()
	sink := &codexJSONLTestSink{}
	line := []byte(`{"type":"response_item","payload":{"type":"function_call_output","output":"exit 0\nfile.txt","call_id":"c1"}}`)
	dispatchCodexJSONL("sess-1", line, sink)

	sink.mu.Lock()
	defer sink.mu.Unlock()
	if len(sink.msgs) != 0 {
		t.Errorf("msgs = %+v, want none (tool_result dropped)", sink.msgs)
	}
	if sink.bumps != 0 {
		t.Errorf("bumps = %d, want 0", sink.bumps)
	}
}

// TestDispatchCodexJSONL_UnknownTypeIgnored verifies records the tailer
// doesn't recognize (session_meta, turn_context, response_item/message,
// response_item/reasoning, event_msg/user_message, event_msg/task_started)
// fall through silently — no spurious bumps or message rows.
func TestDispatchCodexJSONL_UnknownTypeIgnored(t *testing.T) {
	t.Parallel()
	sink := &codexJSONLTestSink{}
	for _, line := range []string{
		`{"type":"session_meta","payload":{"id":"x"}}`,
		`{"type":"turn_context","payload":{"cwd":"/tmp"}}`,
		`{"type":"response_item","payload":{"type":"message","role":"user","content":[]}}`,
		`{"type":"response_item","payload":{"type":"reasoning","summary":[]}}`,
		`{"type":"event_msg","payload":{"type":"user_message"}}`,
		`{"type":"event_msg","payload":{"type":"task_started","model_context_window":258400}}`,
		`not even valid json`,
	} {
		dispatchCodexJSONL("sess-1", []byte(line), sink)
	}
	sink.mu.Lock()
	defer sink.mu.Unlock()
	if len(sink.msgs) != 0 || sink.bumps != 0 || len(sink.contextUpdates) != 0 {
		t.Errorf("expected all-zero, got msgs=%d bumps=%d ctx=%d", len(sink.msgs), sink.bumps, len(sink.contextUpdates))
	}
}

// TestReadNewLines_RespectsOffset verifies the file-poll helper consumes only
// complete newline-terminated lines beyond startOffset, and leaves a partial
// trailing line unconsumed for the next call.
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

// TestWaitForRolloutFile_FindsFileMatchingCwd verifies the discovery loop
// returns the rollout path whose session_meta.payload.cwd matches our
// resolved workdir. Codex 0.130 writes to ~/.codex/sessions/ regardless of
// CODEX_HOME, so identity must come from session_meta.cwd, not the path.
func TestWaitForRolloutFile_FindsFileMatchingCwd(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	dir := filepath.Join(fakeHome, ".codex", "sessions", "2026", "05", "12")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	workDir := "/private/var/folders/x/projects/p-codex-1"

	// Decoy: another rollout for a different cwd.
	decoy := filepath.Join(dir, "rollout-2026-05-12T00-00-00-decoy.jsonl")
	if err := os.WriteFile(decoy, []byte(`{"type":"session_meta","payload":{"cwd":"/other/dir"}}`+"\n"), 0o600); err != nil {
		t.Fatalf("decoy write: %v", err)
	}

	// Real: created async after ~100ms with matching cwd.
	wanted := filepath.Join(dir, "rollout-2026-05-12T00-00-00-real.jsonl")
	go func() {
		time.Sleep(100 * time.Millisecond)
		body := []byte(`{"type":"session_meta","payload":{"cwd":"` + workDir + `"}}` + "\n")
		_ = os.WriteFile(wanted, body, 0o600)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	path, err := waitForRolloutFile(ctx, workDir, time.Now(), 2*time.Second)
	if err != nil {
		t.Fatalf("waitForRolloutFile: %v", err)
	}
	if !strings.HasSuffix(path, "rollout-2026-05-12T00-00-00-real.jsonl") {
		t.Errorf("path = %q, want suffix rollout-2026-05-12T00-00-00-real.jsonl (decoy must not match)", path)
	}
}

// TestWaitForRolloutFile_DeadlineExceeded verifies that no matching file
// (only decoys with wrong cwd) returns an error after the deadline.
func TestWaitForRolloutFile_DeadlineExceeded(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	dir := filepath.Join(fakeHome, ".codex", "sessions", "2026", "05", "12")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Decoy that should NOT match.
	if err := os.WriteFile(filepath.Join(dir, "rollout-decoy.jsonl"),
		[]byte(`{"type":"session_meta","payload":{"cwd":"/other"}}`+"\n"), 0o600); err != nil {
		t.Fatalf("decoy: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	if _, err := waitForRolloutFile(ctx, "/private/var/folders/x/p-codex-1", time.Now(), 200*time.Millisecond); err == nil {
		t.Error("waitForRolloutFile returned nil error despite no matching cwd")
	}
}
