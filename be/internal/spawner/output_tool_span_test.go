package spawner

import (
	"strings"
	"testing"
	"time"

	"be/internal/clock"
)

// TestTrackToolInvoke_PayloadCarriesToolUseID verifies the invoke row payload.
func TestTrackToolInvoke_PayloadCarriesToolUseID(t *testing.T) {
	t.Parallel()
	s := New(Config{Clock: clock.NewTest(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))})
	proc := &processInfo{}

	s.TrackToolInvoke(proc, "[Bash] ls", "tool", "tu_1", []byte(`{"command":"ls"}`))
	s.TrackToolInvoke(proc, "[Edit] main.go", "tool", "", []byte(`{"file_path":"main.go"}`)) // no id → plain row

	if len(proc.pendingMessages) != 2 {
		t.Fatalf("pendingMessages = %d, want 2", len(proc.pendingMessages))
	}
	if !strings.Contains(proc.pendingMessages[0].Payload, `"tool_use_id":"tu_1"`) {
		t.Errorf("payload = %q, want tool_use_id", proc.pendingMessages[0].Payload)
	}
	if !strings.Contains(proc.pendingMessages[0].Payload, `"input":{"command":"ls"}`) {
		t.Errorf("payload = %q, want raw input embedded", proc.pendingMessages[0].Payload)
	}
	if proc.pendingMessages[1].Payload != "" {
		t.Errorf("empty id should produce empty payload, got %q", proc.pendingMessages[1].Payload)
	}
}

// TestCloseToolSpan_StampsPendingBufferEntry verifies fast tools are closed
// in memory before the next flush.
func TestCloseToolSpan_StampsPendingBufferEntry(t *testing.T) {
	t.Parallel()
	s := New(Config{Clock: clock.NewTest(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))})
	proc := &processInfo{}

	s.TrackToolInvoke(proc, "[Bash] ls", "tool", "tu_1", []byte(`{"command":"ls"}`))
	s.CloseToolSpan(proc, "tu_1")

	if !strings.Contains(proc.pendingMessages[0].Payload, `"ended_at":"2025-01-01T00:00:00Z"`) {
		t.Errorf("payload = %q, want stamped ended_at", proc.pendingMessages[0].Payload)
	}
	// stampPendingToolEnd unmarshals into map[string]any (not map[string]string)
	// specifically so the nested "input" object survives the stamp round-trip.
	if !strings.Contains(proc.pendingMessages[0].Payload, `"input":{"command":"ls"}`) {
		t.Errorf("payload = %q, want input preserved alongside ended_at", proc.pendingMessages[0].Payload)
	}
	// Second close is a no-op (span already closed; nothing matches).
	s.CloseToolSpan(proc, "tu_1")
	if n := strings.Count(proc.pendingMessages[0].Payload, "ended_at"); n != 1 {
		t.Errorf("ended_at stamped %d times, want 1", n)
	}
}

// TestCloseToolSpan_FallsBackToDBRow verifies slow tools whose invoke row was
// already flushed get closed via repo.SetToolEnded.
func TestCloseToolSpan_FallsBackToDBRow(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()
	env.createSession(t, "claude:sonnet-5")
	proc := &processInfo{sessionID: env.sessionID}

	env.spawner.TrackToolInvoke(proc, "[WebFetch] http://x", "tool", "tu_slow", []byte(`{"url":"http://x"}`))
	env.spawner.saveMessages(proc) // flush → row in DB, buffer empty

	env.spawner.CloseToolSpan(proc, "tu_slow")

	var payload string
	if err := env.database.QueryRow(
		`SELECT payload FROM agent_messages WHERE session_id = ?`, env.sessionID,
	).Scan(&payload); err != nil {
		t.Fatalf("query payload: %v", err)
	}
	if !strings.Contains(payload, `"ended_at"`) || !strings.Contains(payload, `"tool_use_id":"tu_slow"`) {
		t.Errorf("payload = %q, want tool_use_id + ended_at", payload)
	}
}
