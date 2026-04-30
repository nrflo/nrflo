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
