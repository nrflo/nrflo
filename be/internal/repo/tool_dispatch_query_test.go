package repo

import (
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/model"
)

func setupDispatchQueryDB(t *testing.T) (*DispatchRepo, string) {
	t.Helper()
	r, _, projectID := setupDispatchQueryDBWithClock(t)
	return r, projectID
}

// setupDispatchQueryDBWithClock also returns the TestClock so callers can
// Advance() between inserts to get distinct created_at orderings.
func setupDispatchQueryDBWithClock(t *testing.T) (*DispatchRepo, *clock.TestClock, string) {
	t.Helper()
	pool := newTestPool(t)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	mustExec(t, pool, `INSERT INTO projects (id, name, created_at, updated_at) VALUES ('proj-dq', 'P', ?, ?)`, now, now)
	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	return NewDispatchRepo(pool, clk), clk, "proj-dq"
}

func insertDispatch(t *testing.T, r *DispatchRepo, sessionID, toolName, status string) {
	t.Helper()
	sid := sessionID
	d := &model.ToolDispatch{
		ProjectID: "proj-dq",
		SessionID: &sid,
		ToolName:  toolName,
		Input:     `{}`,
		Status:    status,
	}
	if err := r.Insert(d); err != nil {
		t.Fatalf("Insert: %v", err)
	}
}

func TestListBySession_ScopedAndNewestFirst(t *testing.T) {
	t.Parallel()
	r, clk, _ := setupDispatchQueryDBWithClock(t)
	insertDispatch(t, r, "sess-1", "tool_a", model.DispatchStatusSuccess)
	clk.Advance(time.Second)
	insertDispatch(t, r, "sess-1", "tool_b", model.DispatchStatusSuccess)
	clk.Advance(time.Second)
	insertDispatch(t, r, "sess-2", "tool_c", model.DispatchStatusSuccess)

	rows, err := r.ListBySession("sess-1", 0)
	if err != nil {
		t.Fatalf("ListBySession: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("len(rows) = %d, want 2 (scoped to sess-1)", len(rows))
	}
	// Newest first: tool_b was inserted after tool_a.
	if rows[0].ToolName != "tool_b" || rows[1].ToolName != "tool_a" {
		t.Errorf("order = %s,%s, want tool_b,tool_a (created_at DESC)", rows[0].ToolName, rows[1].ToolName)
	}
}

func TestListBySession_LimitHonored(t *testing.T) {
	t.Parallel()
	r, _ := setupDispatchQueryDB(t)
	for i := 0; i < 5; i++ {
		insertDispatch(t, r, "sess-lim", "tool", model.DispatchStatusSuccess)
	}
	rows, err := r.ListBySession("sess-lim", 2)
	if err != nil {
		t.Fatalf("ListBySession: %v", err)
	}
	if len(rows) != 2 {
		t.Errorf("len(rows) = %d, want 2", len(rows))
	}
}

func TestToolDistribution_AggregatesSuccessAndErrorPerTool(t *testing.T) {
	t.Parallel()
	r, _ := setupDispatchQueryDB(t)
	insertDispatch(t, r, "sess-a", "read_file", model.DispatchStatusSuccess)
	insertDispatch(t, r, "sess-a", "read_file", model.DispatchStatusSuccess)
	insertDispatch(t, r, "sess-b", "read_file", model.DispatchStatusError)
	insertDispatch(t, r, "sess-b", "write_file", model.DispatchStatusSuccess)
	// Not in the requested session set — must not leak into the distribution.
	insertDispatch(t, r, "sess-excluded", "read_file", model.DispatchStatusSuccess)

	stats, err := r.ToolDistribution([]string{"sess-a", "sess-b"})
	if err != nil {
		t.Fatalf("ToolDistribution: %v", err)
	}
	byKey := map[string]int{}
	for _, s := range stats {
		byKey[s.ToolName+"/"+s.Status] = s.Count
	}
	if byKey["read_file/success"] != 2 {
		t.Errorf("read_file/success = %d, want 2", byKey["read_file/success"])
	}
	if byKey["read_file/error"] != 1 {
		t.Errorf("read_file/error = %d, want 1", byKey["read_file/error"])
	}
	if byKey["write_file/success"] != 1 {
		t.Errorf("write_file/success = %d, want 1", byKey["write_file/success"])
	}
}

func TestToolDistribution_EmptySessionIDs_ReturnsNil(t *testing.T) {
	t.Parallel()
	r, _ := setupDispatchQueryDB(t)
	insertDispatch(t, r, "sess-any", "tool", model.DispatchStatusSuccess)

	stats, err := r.ToolDistribution(nil)
	if err != nil {
		t.Fatalf("ToolDistribution: %v", err)
	}
	if stats != nil {
		t.Errorf("stats = %+v, want nil (empty sessionIDs must not return every row in the table)", stats)
	}
}

func TestDeleteBefore_CutoffBoundary(t *testing.T) {
	t.Parallel()
	r, projectID := setupDispatchQueryDB(t)

	// Insert three rows with explicit created_at values straddling the cutoff.
	rows := []struct {
		id        string
		createdAt string
	}{
		{"disp-old", "2025-12-30T00:00:00Z"},
		{"disp-boundary", "2025-12-31T00:00:00Z"},
		{"disp-new", "2026-01-01T00:00:00Z"},
	}
	for _, row := range rows {
		if _, err := r.db.Exec(`INSERT INTO tool_dispatches (id, project_id, session_id, tool_name, input, output, status, error_msg, duration_ms, source, session_kind, workflow_instance_id, created_at)
			VALUES (?, ?, NULL, 'tool', '{}', NULL, 'success', NULL, 0, '', '', '', ?)`, row.id, projectID, row.createdAt); err != nil {
			t.Fatalf("seed dispatch %s: %v", row.id, err)
		}
	}

	// Cutoff excludes disp-boundary itself (< cutoff, not <=) and disp-old.
	deleted, err := r.DeleteBefore("2025-12-31T00:00:00Z")
	if err != nil {
		t.Fatalf("DeleteBefore: %v", err)
	}
	if deleted != 1 {
		t.Errorf("deleted = %d, want 1 (only disp-old, strictly before cutoff)", deleted)
	}

	var remaining int
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM tool_dispatches`).Scan(&remaining); err != nil {
		t.Fatalf("count remaining: %v", err)
	}
	if remaining != 2 {
		t.Errorf("remaining = %d, want 2 (disp-boundary and disp-new survive)", remaining)
	}
}
