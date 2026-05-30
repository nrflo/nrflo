package socket

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"be/internal/clock"
	"be/internal/ws"
)

// TestTailThinking_AppendNewLines verifies that after the first tailThinking call,
// appending new lines and calling again inserts only the new rows.
func TestTailThinking_AppendNewLines(t *testing.T) {
	env, h, sessionID := setupThinkingSession(t, "TH-APPEND")
	path := filepath.Join(t.TempDir(), "transcript.jsonl")

	f, _ := os.Create(path)
	fmt.Fprintf(f, "%s\n", thinkingLine("initial"))
	f.Close()

	h.tailThinking(context.Background(), sessionID, path)
	if n := countAgentMessages(t, env, sessionID); n != 1 {
		t.Fatalf("after first call: want 1 row, got %d", n)
	}

	f, _ = os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	fmt.Fprintf(f, "%s\n", thinkingLine("appended"))
	f.Close()

	h.tailThinking(context.Background(), sessionID, path)
	if n := countAgentMessages(t, env, sessionID); n != 2 {
		t.Errorf("after append+second call: want 2 rows, got %d", n)
	}
}

// TestTailThinking_PartialLine verifies that a line without a terminating \n is not
// consumed; after completing the line with \n it is consumed exactly once (no dup).
func TestTailThinking_PartialLine(t *testing.T) {
	env, h, sessionID := setupThinkingSession(t, "TH-PARTIAL")
	path := filepath.Join(t.TempDir(), "transcript.jsonl")

	partial := thinkingLine("partial-thought")
	f, _ := os.Create(path)
	fmt.Fprintf(f, "%s\n", thinkingLine("complete"))
	f.Write(partial) // no trailing newline
	f.Close()

	h.tailThinking(context.Background(), sessionID, path)
	if n := countAgentMessages(t, env, sessionID); n != 1 {
		t.Fatalf("partial line: want 1 row (complete line only), got %d", n)
	}

	f, _ = os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	f.Write([]byte("\n"))
	f.Close()

	h.tailThinking(context.Background(), sessionID, path)
	if n := countAgentMessages(t, env, sessionID); n != 2 {
		t.Errorf("after completing partial line: want 2 rows, got %d", n)
	}
}

// TestTailThinking_Rotation verifies that when the stored offset exceeds the
// current file size (file rotated/rewritten smaller), the offset resets to 0
// and the new content is read from the beginning.
func TestTailThinking_Rotation(t *testing.T) {
	env, h, sessionID := setupThinkingSession(t, "TH-ROTATE")
	path := filepath.Join(t.TempDir(), "transcript.jsonl")

	os.WriteFile(path, append(thinkingLine("before-rotation"), '\n'), 0644)
	h.tailThinking(context.Background(), sessionID, path)
	if n := countAgentMessages(t, env, sessionID); n != 1 {
		t.Fatalf("before rotation: want 1 row, got %d", n)
	}

	// Rewrite file with new content (simulates rotation / shrink).
	os.WriteFile(path, append(thinkingLine("after-rotation"), '\n'), 0644)

	h.tailThinking(context.Background(), sessionID, path)
	if n := countAgentMessages(t, env, sessionID); n != 2 {
		t.Errorf("after rotation: want 2 rows total, got %d", n)
	}
}

// TestTailThinking_Truncation verifies that a thinking block larger than 256 KiB
// is stored with content truncated to ~256 KiB plus the ellipsis marker suffix.
func TestTailThinking_Truncation(t *testing.T) {
	env, h, sessionID := setupThinkingSession(t, "TH-TRUNC")
	bigText := strings.Repeat("x", maxThinkingBytes+1024)
	path := filepath.Join(t.TempDir(), "transcript.jsonl")
	f, _ := os.Create(path)
	fmt.Fprintf(f, "%s\n", thinkingLine(bigText))
	f.Close()

	h.tailThinking(context.Background(), sessionID, path)

	if n := countAgentMessages(t, env, sessionID); n != 1 {
		t.Fatalf("truncation: want 1 row, got %d", n)
	}
	content, category := lastAgentMessage(t, env, sessionID)
	if category != "thinking" {
		t.Errorf("category = %q, want %q", category, "thinking")
	}
	if len(content) > maxThinkingBytes+50 {
		t.Errorf("stored content len %d exceeds truncation limit + marker overhead", len(content))
	}
	if !strings.HasSuffix(content, "\n…[truncated]") {
		t.Errorf("truncated content must end with ellipsis marker, suffix = %q", content[max(0, len(content)-20):])
	}
}

// TestPreToolUse_ThinkingSeqOrdering verifies the full PreToolUse integration:
// thinking rows get a lower seq than the tool row, category is "thinking" with
// no "[thinking]" prefix, and a messages.updated broadcast is sent.
func TestPreToolUse_ThinkingSeqOrdering(t *testing.T) {
	env := newHandlerTestEnv(t)
	env.createTicketAndWorkflow(t, "TH-SEQ")
	wfiID := queryWFIID(t, env, "TH-SEQ")
	sessionID := "sess-think-seq"
	insertAgentSession(t, env, "TH-SEQ", sessionID, wfiID)
	enableThinking(t, env)

	client, sendCh := ws.NewTestClient(env.hub, "th-seq-client")
	env.hub.Subscribe(client, env.project, "TH-SEQ")

	transcriptPath := filepath.Join(t.TempDir(), "transcript.jsonl")
	f, _ := os.Create(transcriptPath)
	fmt.Fprintf(f, "%s\n", thinkingLine("pre-tool thought"))
	f.Close()

	sig := &bumpRecordSignaler{}
	h := NewHandler(env.pool, env.hub, clock.Real(), sig)

	req := buildRecordEventReq(t, "req-th-seq", sessionID, map[string]interface{}{
		"hook_event_name": "PreToolUse",
		"tool_name":       "Bash",
		"tool_input":      map[string]interface{}{"command": "ls"},
		"transcript_path": transcriptPath,
	})
	resp := h.Handle(req)
	if resp.Error != nil {
		t.Fatalf("PreToolUse: unexpected error: %v", resp.Error)
	}

	if n := countAgentMessages(t, env, sessionID); n != 2 {
		t.Fatalf("want 2 agent_messages rows (thinking+tool), got %d", n)
	}

	// Verify seq ordering: thinking row must have lower seq than tool row.
	rows, err := env.pool.Query(
		`SELECT category, seq FROM agent_messages WHERE session_id = ? ORDER BY seq ASC`,
		sessionID,
	)
	if err != nil {
		t.Fatalf("query agent_messages: %v", err)
	}
	defer rows.Close()
	type msgRow struct {
		category string
		seq      int
	}
	var got []msgRow
	for rows.Next() {
		var r msgRow
		if err := rows.Scan(&r.category, &r.seq); err != nil {
			t.Fatalf("scan: %v", err)
		}
		got = append(got, r)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(got))
	}
	if got[0].category != "thinking" {
		t.Errorf("row[0].category = %q, want thinking", got[0].category)
	}
	if got[1].category != "tool" {
		t.Errorf("row[1].category = %q, want tool", got[1].category)
	}
	if got[0].seq >= got[1].seq {
		t.Errorf("thinking seq %d >= tool seq %d; thinking must render above tool call", got[0].seq, got[1].seq)
	}

	// Verify thinking content has no "[thinking]" prefix.
	var thinkContent string
	env.pool.QueryRow(
		`SELECT content FROM agent_messages WHERE session_id = ? AND category = 'thinking'`,
		sessionID,
	).Scan(&thinkContent)
	if strings.HasPrefix(thinkContent, "[thinking]") {
		t.Errorf("thinking content must not have [thinking] prefix, got: %q", thinkContent)
	}
	if thinkContent != "pre-tool thought" {
		t.Errorf("thinking content = %q, want %q", thinkContent, "pre-tool thought")
	}

	// At least one messages.updated broadcast must have fired (for the thinking drain).
	ev := awaitWSEvent(t, sendCh, ws.EventMessagesUpdated)
	if sid, _ := ev.Data["session_id"].(string); sid != sessionID {
		t.Errorf("broadcast session_id = %q, want %q", sid, sessionID)
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
