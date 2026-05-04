package service

import (
	"path/filepath"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"
)

func setupInsightsDB(t *testing.T) (*db.Pool, string, *clock.TestClock) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "insights_test.db")
	if err := svcCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	clk := clock.NewTest(time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC))
	_, err = pool.Exec(`INSERT INTO projects (id, name, created_at, updated_at)
		VALUES ('proj-ins', 'Insights', datetime('now'), datetime('now'))`)
	if err != nil {
		t.Fatalf("insert project: %v", err)
	}
	return pool, "proj-ins", clk
}

func insertDispatch(t *testing.T, pool *db.Pool, clk clock.Clock, projectID, toolName, status string, durationMs int64) {
	t.Helper()
	d := &model.NrvappToolDispatch{
		ProjectID:  projectID,
		ToolName:   toolName,
		Input:      `{"x":1}`,
		Status:     status,
		DurationMs: durationMs,
	}
	r := repo.NewNrvappDispatchRepo(pool, clk)
	if err := r.Insert(d); err != nil {
		t.Fatalf("insertDispatch: %v", err)
	}
}

func insertDispatchWithSession(t *testing.T, pool *db.Pool, clk clock.Clock,
	projectID, sessionID, toolName, status string) string {
	t.Helper()
	sid := sessionID
	d := &model.NrvappToolDispatch{
		ProjectID:  projectID,
		SessionID:  &sid,
		ToolName:   toolName,
		Input:      `{"x":1}`,
		Status:     status,
		DurationMs: 100,
	}
	r := repo.NewNrvappDispatchRepo(pool, clk)
	if err := r.Insert(d); err != nil {
		t.Fatalf("insertDispatchWithSession: %v", err)
	}
	return d.ID
}

func insertReviewItem(t *testing.T, pool *db.Pool, clk clock.Clock,
	projectID, sessionID, toolName, inputStr string, status string, draftStr *string) {
	t.Helper()
	sid := sessionID
	item := &model.NrvappReviewItem{
		ProjectID: projectID,
		SessionID: &sid,
		ToolName:  toolName,
		Input:     inputStr,
		Draft:     draftStr,
	}
	r := repo.NewNrvappReviewRepo(pool, clk)
	if err := r.Insert(item); err != nil {
		t.Fatalf("insertReviewItem: %v", err)
	}
	if status == model.ReviewStatusApproved {
		if err := r.Approve(item.ID, projectID); err != nil {
			t.Fatalf("Approve: %v", err)
		}
	} else if status == model.ReviewStatusRejected {
		if err := r.Reject(item.ID, projectID, "test reason"); err != nil {
			t.Fatalf("Reject: %v", err)
		}
	}
}

// --- ParseRange ---

func TestParseRange(t *testing.T) {
	t.Parallel()
	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	cases := []struct {
		input   string
		wantErr bool
		wantAgo time.Duration
	}{
		{"7d", false, 7 * 24 * time.Hour},
		{"30d", false, 30 * 24 * time.Hour},
		{"1d", true, 0},
		{"", true, 0},
	}
	for _, c := range cases {
		t, _ := t, c // shadow for parallel-style clarity
		since, err := ParseRange(c.input, clk)
		if c.wantErr {
			if err == nil {
				t.Errorf("ParseRange(%q): want error, got nil", c.input)
			}
		} else {
			if err != nil {
				t.Fatalf("ParseRange(%q): %v", c.input, err)
			}
			got := clk.Now().Sub(since)
			if got != c.wantAgo {
				t.Errorf("ParseRange(%q) duration = %v, want %v", c.input, got, c.wantAgo)
			}
		}
	}
}

// --- ParseBucket ---

func TestParseBucket(t *testing.T) {
	t.Parallel()
	cases := []struct {
		input   string
		want    time.Duration
		wantErr bool
	}{
		{"1h", time.Hour, false},
		{"6h", 6 * time.Hour, false},
		{"1d", 24 * time.Hour, false},
		{"2h", 0, true},
		{"", 0, true},
	}
	for _, c := range cases {
		got, err := ParseBucket(c.input)
		if c.wantErr {
			if err == nil {
				t.Errorf("ParseBucket(%q): want error, got nil", c.input)
			}
		} else {
			if err != nil {
				t.Errorf("ParseBucket(%q): %v", c.input, err)
			}
			if got != c.want {
				t.Errorf("ParseBucket(%q) = %v, want %v", c.input, got, c.want)
			}
		}
	}
}

// --- Summary ---

func TestNrvappInsightsService_Summary(t *testing.T) {
	t.Parallel()
	pool, pid, clk := setupInsightsDB(t)

	// Seed dispatches: 3 success, 2 error with known durations
	for _, d := range []struct {
		status string
		ms     int64
	}{
		{model.DispatchStatusSuccess, 10},
		{model.DispatchStatusSuccess, 20},
		{model.DispatchStatusSuccess, 100},
		{model.DispatchStatusError, 50},
		{model.DispatchStatusError, 200},
	} {
		insertDispatch(t, pool, clk, pid, "tool-a", d.status, d.ms)
	}
	// Seed review items with known statuses
	for _, s := range []string{
		model.ReviewStatusPending,
		model.ReviewStatusApproved,
		model.ReviewStatusApproved,
		model.ReviewStatusRejected,
	} {
		item := &model.NrvappReviewItem{ProjectID: pid, ToolName: "t", Input: "{}"}
		r := repo.NewNrvappReviewRepo(pool, clk)
		r.Insert(item) //nolint
		switch s {
		case model.ReviewStatusApproved:
			r.Approve(item.ID, pid) //nolint
		case model.ReviewStatusRejected:
			r.Reject(item.ID, pid, "") //nolint
		}
	}

	svc := NewNrvappInsightsService(pool, clk)
	since := clk.Now().Add(-7 * 24 * time.Hour)
	summary, err := svc.Summary(pid, since)
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if summary.Total != 5 {
		t.Errorf("Total = %d, want 5", summary.Total)
	}
	if summary.Success != 3 {
		t.Errorf("Success = %d, want 3", summary.Success)
	}
	if summary.Error != 2 {
		t.Errorf("Error = %d, want 2", summary.Error)
	}
	if summary.ReviewPending != 1 {
		t.Errorf("ReviewPending = %d, want 1", summary.ReviewPending)
	}
	if summary.ReviewApproved != 2 {
		t.Errorf("ReviewApproved = %d, want 2", summary.ReviewApproved)
	}
	if summary.ReviewRejected != 1 {
		t.Errorf("ReviewRejected = %d, want 1", summary.ReviewRejected)
	}
	// ApprovalRate = 2/3 ≈ 0.666...
	if summary.ApprovalRate < 0.6 || summary.ApprovalRate > 0.7 {
		t.Errorf("ApprovalRate = %v, want ~0.667", summary.ApprovalRate)
	}
}

// --- EditRate ---

func TestNrvappInsightsService_EditRate(t *testing.T) {
	t.Parallel()
	pool, pid, clk := setupInsightsDB(t)
	const sid = "sess-edit-1"
	const tool = "tool-edit"

	insertDispatchWithSession(t, pool, clk, pid, sid, tool, model.DispatchStatusSuccess)

	// approved with edits (draft differs from input)
	draft1 := `{"x":2}`
	insertReviewItem(t, pool, clk, pid, sid, tool, `{"x":1}`, model.ReviewStatusApproved, &draft1)

	// approved no edits (draft same as input or nil)
	insertReviewItem(t, pool, clk, pid, sid, tool, `{"x":1}`, model.ReviewStatusApproved, nil)

	// rejected
	insertReviewItem(t, pool, clk, pid, sid, tool, `{"x":1}`, model.ReviewStatusRejected, nil)

	svc := NewNrvappInsightsService(pool, clk)
	since := clk.Now().Add(-7 * 24 * time.Hour)
	rows, err := svc.EditRate(pid, since)
	if err != nil {
		t.Fatalf("EditRate: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("EditRate returned empty rows")
	}
	r := rows[0]
	if r.ToolName != tool {
		t.Errorf("ToolName = %q, want %q", r.ToolName, tool)
	}
	if r.ApproveWithEdits != 1 {
		t.Errorf("ApproveWithEdits = %d, want 1", r.ApproveWithEdits)
	}
	if r.ApproveNoEdits != 1 {
		t.Errorf("ApproveNoEdits = %d, want 1", r.ApproveNoEdits)
	}
	if r.Rejected != 1 {
		t.Errorf("Rejected = %d, want 1", r.Rejected)
	}
	// EditRatePct = 1/3 * 100 ≈ 33.3
	if r.EditRatePct < 30 || r.EditRatePct > 40 {
		t.Errorf("EditRatePct = %v, want ~33.3", r.EditRatePct)
	}
}

// --- Throughput ---

func TestNrvappInsightsService_Throughput(t *testing.T) {
	t.Parallel()
	pool, pid, clk := setupInsightsDB(t)

	// Insert 2 dispatches in the same hour bucket, 1 in a different hour.
	insertDispatch(t, pool, clk, pid, "tool-t", model.DispatchStatusSuccess, 10)
	insertDispatch(t, pool, clk, pid, "tool-t", model.DispatchStatusSuccess, 10)
	clk.Advance(2 * time.Hour)
	insertDispatch(t, pool, clk, pid, "tool-t", model.DispatchStatusSuccess, 10)

	svc := NewNrvappInsightsService(pool, clk)
	since := clk.Now().Add(-7 * 24 * time.Hour)
	points, err := svc.Throughput(pid, since, time.Hour)
	if err != nil {
		t.Fatalf("Throughput: %v", err)
	}
	if len(points) < 2 {
		t.Fatalf("Throughput returned %d points, want >= 2", len(points))
	}
	var total int
	for _, p := range points {
		total += p.Count
	}
	if total != 3 {
		t.Errorf("total dispatches in throughput = %d, want 3", total)
	}
}
