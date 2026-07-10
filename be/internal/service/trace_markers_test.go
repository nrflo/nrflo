package service

import (
	"strings"
	"testing"

	"be/internal/db"
	"be/internal/types"
)

func insertTraceMessage(t *testing.T, pool *db.Pool, sessionID string, seq int, content, category, createdAt string) {
	t.Helper()
	mustExec(t, pool, `INSERT INTO agent_messages (session_id, seq, content, category, created_at) VALUES (?, ?, ?, ?, ?)`,
		sessionID, seq, content, category, createdAt)
}

func insertFindingHistory(t *testing.T, pool *db.Pool, id, scope, scopeID, key, op, actorID, createdAt string) {
	t.Helper()
	mustExec(t, pool, `INSERT INTO findings_history (id, scope, scope_id, key, operation, actor_id, actor_source, created_at)
		VALUES (?, ?, ?, ?, ?, NULLIF(?, ''), 'socket', ?)`, id, scope, scopeID, key, op, actorID, createdAt)
}

// setupMarkerEnv seeds one running analyzer session on the trace env.
func setupMarkerEnv(t *testing.T) (*db.Pool, *WorkflowService, string) {
	t.Helper()
	pool, svc, wfiID := setupTraceTestEnv(t)
	insertTraceSession(t, pool, traceSession{id: "s-a", wfiID: wfiID, agentType: "analyzer",
		status: "running", startedAt: "2025-01-01T00:00:00Z"})
	return pool, svc, wfiID
}

func laneMarkers(t *testing.T, trace *types.TraceResponse, laneID string) []types.TraceMarker {
	t.Helper()
	for _, l := range trace.Lanes {
		if l.LaneID == laneID {
			return l.Markers
		}
	}
	t.Fatalf("lane %q not found", laneID)
	return nil
}

func TestBuildTrace_DefaultCategoriesExcludeNoise(t *testing.T) {
	t.Parallel()
	pool, svc, wfiID := setupMarkerEnv(t)
	base := "2025-01-01T00:00:0%dZ"
	for i, m := range []struct{ content, category string }{
		{"[Bash] ls", "tool"}, {"pondering", "thinking"}, {"plain text", "text"},
		{"boom", "error"}, {"user says hi", "user_input"},
	} {
		insertTraceMessage(t, pool, "s-a", i, m.content, m.category, strings.Replace(base, "%d", string(rune('0'+i)), 1))
	}

	trace, err := svc.BuildTrace(wfiID, TraceOptions{})
	if err != nil {
		t.Fatalf("BuildTrace: %v", err)
	}
	markers := laneMarkers(t, trace, "s-a")
	if len(markers) != 3 {
		t.Fatalf("markers = %d, want 3 (tool/error/user_input)", len(markers))
	}
	for _, m := range markers {
		if m.Type == "text" || m.Type == "thinking" {
			t.Errorf("noise category %q leaked into default trace", m.Type)
		}
	}
	if markers[0].Label != "[Bash] ls" {
		t.Errorf("first marker = %q, want earliest tool row", markers[0].Label)
	}
}

func TestBuildTrace_ExplicitCategorySelection(t *testing.T) {
	t.Parallel()
	pool, svc, wfiID := setupMarkerEnv(t)
	insertTraceMessage(t, pool, "s-a", 0, "[Bash] ls", "tool", "2025-01-01T00:00:01Z")
	insertTraceMessage(t, pool, "s-a", 1, "pondering", "thinking", "2025-01-01T00:00:02Z")

	trace, err := svc.BuildTrace(wfiID, TraceOptions{Categories: []string{"thinking"}})
	if err != nil {
		t.Fatalf("BuildTrace: %v", err)
	}
	markers := laneMarkers(t, trace, "s-a")
	if len(markers) != 1 || markers[0].Type != "thinking" {
		t.Fatalf("markers = %+v, want single thinking marker", markers)
	}
}

func TestBuildTrace_MarkerLimitTruncatesEarliestFirst(t *testing.T) {
	t.Parallel()
	pool, svc, wfiID := setupMarkerEnv(t)
	for i := 0; i < 5; i++ {
		insertTraceMessage(t, pool, "s-a", i, "tool "+string(rune('a'+i)), "tool",
			"2025-01-01T00:00:0"+string(rune('0'+i))+"Z")
	}

	trace, err := svc.BuildTrace(wfiID, TraceOptions{MarkerLimit: 2})
	if err != nil {
		t.Fatalf("BuildTrace: %v", err)
	}
	if !trace.Truncated {
		t.Error("expected truncated=true")
	}
	markers := laneMarkers(t, trace, "s-a")
	if len(markers) != 2 || markers[0].Label != "tool a" || markers[1].Label != "tool b" {
		t.Fatalf("markers = %+v, want earliest two (tool a, tool b)", markers)
	}
}

func TestBuildTrace_FindingMarkerAttribution(t *testing.T) {
	t.Parallel()
	pool, svc, wfiID := setupMarkerEnv(t)
	// Session-scope write → analyzer lane.
	insertFindingHistory(t, pool, "fh1", "session", "s-a", "report", "add", "s-a", "2025-01-01T00:00:01Z")
	// Instance-scope write by a known session → analyzer lane.
	insertFindingHistory(t, pool, "fh2", "workflow_instance", wfiID, "summary", "append", "s-a", "2025-01-01T00:00:02Z")
	// Instance-scope write by an unknown actor → root markers.
	insertFindingHistory(t, pool, "fh3", "workflow_instance", wfiID, "seed", "add", "user-1", "2025-01-01T00:00:03Z")

	trace, err := svc.BuildTrace(wfiID, TraceOptions{})
	if err != nil {
		t.Fatalf("BuildTrace: %v", err)
	}
	markers := laneMarkers(t, trace, "s-a")
	if len(markers) != 2 {
		t.Fatalf("lane markers = %+v, want 2 finding markers", markers)
	}
	if markers[0].Type != "finding" || markers[0].Label != "add report" {
		t.Errorf("marker 0 = %+v, want finding 'add report'", markers[0])
	}
	if len(trace.RootMarkers) != 1 || trace.RootMarkers[0].Label != "add seed" {
		t.Fatalf("root markers = %+v, want single 'add seed'", trace.RootMarkers)
	}
	// Findings excluded when categories omit "finding".
	trace2, _ := svc.BuildTrace(wfiID, TraceOptions{Categories: []string{"tool"}})
	if len(laneMarkers(t, trace2, "s-a")) != 0 || len(trace2.RootMarkers) != 0 {
		t.Error("finding markers leaked despite categories=tool")
	}
}

func TestBuildTrace_LabelTruncatedTo200(t *testing.T) {
	t.Parallel()
	pool, svc, wfiID := setupMarkerEnv(t)
	insertTraceMessage(t, pool, "s-a", 0, strings.Repeat("x", 300), "tool", "2025-01-01T00:00:01Z")

	trace, err := svc.BuildTrace(wfiID, TraceOptions{})
	if err != nil {
		t.Fatalf("BuildTrace: %v", err)
	}
	markers := laneMarkers(t, trace, "s-a")
	if len(markers) != 1 || len(markers[0].Label) != 200 {
		t.Fatalf("label length = %d, want 200", len(markers[0].Label))
	}
}

func TestBuildTrace_ToolSpanEndedAt(t *testing.T) {
	t.Parallel()
	pool, svc, wfiID := setupMarkerEnv(t)
	// Closed span: payload carries tool_use_id + ended_at (stamped by PostToolUse).
	mustExec(t, pool, `INSERT INTO agent_messages (session_id, seq, content, category, created_at, payload)
		VALUES ('s-a', 0, '[Bash] make test', 'tool', '2025-01-01T00:00:01Z', '{"tool_use_id":"tu1","ended_at":"2025-01-01T00:00:20Z"}')`)
	// Open span: pre-row only.
	mustExec(t, pool, `INSERT INTO agent_messages (session_id, seq, content, category, created_at, payload)
		VALUES ('s-a', 1, '[Edit] main.go', 'tool', '2025-01-01T00:00:30Z', '{"tool_use_id":"tu2"}')`)

	trace, err := svc.BuildTrace(wfiID, TraceOptions{})
	if err != nil {
		t.Fatalf("BuildTrace: %v", err)
	}
	markers := laneMarkers(t, trace, "s-a")
	if len(markers) != 2 {
		t.Fatalf("markers = %d, want 2", len(markers))
	}
	if markers[0].EndedAt == nil || *markers[0].EndedAt != "2025-01-01T00:00:20Z" {
		t.Errorf("closed span ended_at = %v, want 2025-01-01T00:00:20Z", markers[0].EndedAt)
	}
	if markers[1].EndedAt != nil {
		t.Errorf("open span should have nil ended_at, got %v", markers[1].EndedAt)
	}
}

func TestParseTraceOptions(t *testing.T) {
	t.Parallel()
	opts, err := ParseTraceOptions("", "")
	if err != nil || opts.MarkerLimit != 2000 || len(opts.Categories) != len(traceDefaultCategories) {
		t.Fatalf("defaults = %+v err=%v", opts, err)
	}
	opts, err = ParseTraceOptions("tool, error", "10")
	if err != nil || len(opts.Categories) != 2 || opts.MarkerLimit != 10 {
		t.Fatalf("parsed = %+v err=%v", opts, err)
	}
	for _, bad := range [][2]string{{"bogus", ""}, {",", ""}, {"", "0"}, {"", "9999"}, {"", "nan"}} {
		if _, err := ParseTraceOptions(bad[0], bad[1]); err == nil {
			t.Errorf("ParseTraceOptions(%q, %q) should error", bad[0], bad[1])
		}
	}
}
