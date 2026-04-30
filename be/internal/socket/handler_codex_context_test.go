package socket

import (
	"os"
	"path/filepath"
	"testing"
)

const codexFixtureJSONL = `{"timestamp":"2026-04-30T06:48:35.000Z","type":"session_meta","payload":{"id":"019ddd25-7e89-7332-b095-a11e94fcaffd"}}
{"timestamp":"2026-04-30T06:48:45.362Z","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":11856,"cached_input_tokens":3456,"output_tokens":190,"reasoning_output_tokens":31,"total_tokens":12046},"last_token_usage":{"input_tokens":11856,"cached_input_tokens":3456,"output_tokens":190,"reasoning_output_tokens":31,"total_tokens":12046},"model_context_window":258400}}}
{"timestamp":"2026-04-30T06:48:48.000Z","type":"event_msg","payload":{"type":"task_started"}}
{"timestamp":"2026-04-30T06:48:49.631Z","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":30245,"cached_input_tokens":15104,"output_tokens":380,"reasoning_output_tokens":75,"total_tokens":30625},"last_token_usage":{"input_tokens":18389,"cached_input_tokens":11648,"output_tokens":190,"reasoning_output_tokens":44,"total_tokens":18579},"model_context_window":258400}}}
{"timestamp":"2026-04-30T06:48:49.700Z","type":"event_msg","payload":{"type":"task_complete"}}
`

// TestExtractCodexContextLeft_LatestTurn verifies that the parser uses the
// most recent token_count record's last_token_usage.input_tokens against
// model_context_window, not earlier turns or the cumulative total.
func TestExtractCodexContextLeft_LatestTurn(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rollout.jsonl")
	if err := os.WriteFile(path, []byte(codexFixtureJSONL), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	event := map[string]interface{}{"transcript_path": path}

	pct, ok := extractCodexContextLeft(event)
	if !ok {
		t.Fatal("extractCodexContextLeft() returned ok=false on a valid fixture")
	}
	// last input_tokens=18389, model_context_window=258400
	// pct_left = 100 - 100*18389/258400 = 100 - 7 = 93
	want := 93
	if pct != want {
		t.Errorf("pct = %d, want %d (formula: 100 - 100*18389/258400)", pct, want)
	}
}

func TestExtractCodexContextLeft_NoTranscriptPath(t *testing.T) {
	if _, ok := extractCodexContextLeft(map[string]interface{}{}); ok {
		t.Error("expected ok=false when transcript_path is absent (Claude hook payload)")
	}
}

func TestExtractCodexContextLeft_UnreadableFile(t *testing.T) {
	event := map[string]interface{}{"transcript_path": "/nonexistent-file-xyz.jsonl"}
	if _, ok := extractCodexContextLeft(event); ok {
		t.Error("expected ok=false when transcript_path is unreadable")
	}
}

func TestExtractCodexContextLeft_NoTokenCountRecord(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rollout.jsonl")
	body := `{"type":"session_meta","payload":{"id":"x"}}` + "\n" +
		`{"type":"event_msg","payload":{"type":"task_started"}}` + "\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	event := map[string]interface{}{"transcript_path": path}
	if _, ok := extractCodexContextLeft(event); ok {
		t.Error("expected ok=false when JSONL has no token_count record")
	}
}

// =============================================================================
// extractCodexNewAgentMessages tests
// =============================================================================

const codexAgentMessagesFixture = `{"type":"session_meta","payload":{"id":"x"}}
{"type":"event_msg","payload":{"type":"task_started"}}
{"type":"event_msg","payload":{"type":"agent_message","message":"first commentary","phase":"commentary"}}
{"type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":100},"model_context_window":1000}}}
{"type":"event_msg","payload":{"type":"agent_message","message":"second message","phase":"final_answer"}}
{"type":"response_item","payload":{"type":"reasoning","encrypted_content":"opaque-blob"}}
{"type":"event_msg","payload":{"type":"task_complete"}}
`

func TestExtractCodexNewAgentMessages_ScansFromOffsetZero(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rollout.jsonl")
	if err := os.WriteFile(path, []byte(codexAgentMessagesFixture), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	msgs, newOffset, err := extractCodexNewAgentMessages(path, 0)
	if err != nil {
		t.Fatalf("extractCodexNewAgentMessages() error: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("got %d messages, want 2: %#v", len(msgs), msgs)
	}
	if msgs[0] != "first commentary" || msgs[1] != "second message" {
		t.Errorf("messages = %#v, want [first commentary, second message]", msgs)
	}
	if int(newOffset) != len(codexAgentMessagesFixture) {
		t.Errorf("newOffset = %d, want %d (file size)", newOffset, len(codexAgentMessagesFixture))
	}
}

func TestExtractCodexNewAgentMessages_SkipsAlreadyReadBytes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rollout.jsonl")
	if err := os.WriteFile(path, []byte(codexAgentMessagesFixture), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	// First scan reads everything.
	_, off1, err := extractCodexNewAgentMessages(path, 0)
	if err != nil {
		t.Fatalf("first scan error: %v", err)
	}
	// Second scan from end-of-file: nothing new.
	msgs, off2, err := extractCodexNewAgentMessages(path, off1)
	if err != nil {
		t.Fatalf("second scan error: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("re-scan from end returned %d messages, want 0: %#v", len(msgs), msgs)
	}
	if off2 != off1 {
		t.Errorf("offset moved unexpectedly: %d -> %d", off1, off2)
	}
}

func TestExtractCodexNewAgentMessages_AppendedTurnEmitsOnlyNew(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rollout.jsonl")
	if err := os.WriteFile(path, []byte(codexAgentMessagesFixture), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	_, off1, err := extractCodexNewAgentMessages(path, 0)
	if err != nil {
		t.Fatalf("first scan error: %v", err)
	}

	// Append a new turn with one fresh agent_message.
	more := `{"type":"event_msg","payload":{"type":"agent_message","message":"third turn body"}}` + "\n"
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("reopen for append: %v", err)
	}
	if _, err := f.Write([]byte(more)); err != nil {
		t.Fatalf("append: %v", err)
	}
	f.Close()

	msgs, _, err := extractCodexNewAgentMessages(path, off1)
	if err != nil {
		t.Fatalf("second scan error: %v", err)
	}
	if len(msgs) != 1 || msgs[0] != "third turn body" {
		t.Errorf("appended scan = %#v, want [third turn body]", msgs)
	}
}

func TestExtractCodexNewAgentMessages_ResetOnTruncate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rollout.jsonl")
	if err := os.WriteFile(path, []byte(codexAgentMessagesFixture), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	bigOffset := int64(len(codexAgentMessagesFixture) * 10)
	msgs, newOffset, err := extractCodexNewAgentMessages(path, bigOffset)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("rotated-file scan returned messages: %#v", msgs)
	}
	if newOffset != 0 {
		t.Errorf("expected offset reset to 0 on truncation, got %d", newOffset)
	}
}

func TestExtractCodexNewAgentMessages_IgnoresReasoningRecords(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rollout.jsonl")
	body := `{"type":"response_item","payload":{"type":"reasoning","encrypted_content":"blob"}}` + "\n" +
		`{"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"assistant text"}]}}` + "\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	msgs, _, err := extractCodexNewAgentMessages(path, 0)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("must skip reasoning + response_item/message rows; got %#v", msgs)
	}
}

func TestExtractCodexNewAgentMessages_EmptyPath(t *testing.T) {
	msgs, off, err := extractCodexNewAgentMessages("", 42)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(msgs) != 0 {
		t.Error("empty path must return zero messages")
	}
	if off != 42 {
		t.Errorf("empty path must preserve startOffset; got %d", off)
	}
}

func TestExtractCodexContextLeft_ZeroContextWindow(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rollout.jsonl")
	body := `{"type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":1000},"model_context_window":0}}}` + "\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	event := map[string]interface{}{"transcript_path": path}
	if _, ok := extractCodexContextLeft(event); ok {
		t.Error("expected ok=false when model_context_window is zero")
	}
}
