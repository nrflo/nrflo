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
	d := &model.ToolDispatch{
		ProjectID:  projectID,
		ToolName:   toolName,
		Input:      `{"x":1}`,
		Status:     status,
		DurationMs: durationMs,
	}
	r := repo.NewDispatchRepo(pool, clk)
	if err := r.Insert(d); err != nil {
		t.Fatalf("insertDispatch: %v", err)
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

func TestInsightsService_Summary(t *testing.T) {
	t.Parallel()
	pool, pid, clk := setupInsightsDB(t)

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

	svc := NewInsightsService(pool, clk)
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
}

// --- EditRate ---

func TestInsightsService_EditRate_ReturnsEmpty(t *testing.T) {
	t.Parallel()
	pool, pid, clk := setupInsightsDB(t)
	svc := NewInsightsService(pool, clk)
	since := clk.Now().Add(-7 * 24 * time.Hour)
	rows, err := svc.EditRate(pid, since)
	if err != nil {
		t.Fatalf("EditRate: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("EditRate = %d rows, want 0 (review_items dropped in migration 114)", len(rows))
	}
}

// --- Throughput ---

func TestInsightsService_Throughput(t *testing.T) {
	t.Parallel()
	pool, pid, clk := setupInsightsDB(t)

	// Insert 2 dispatches in the same hour bucket, 1 in a different hour.
	insertDispatch(t, pool, clk, pid, "tool-t", model.DispatchStatusSuccess, 10)
	insertDispatch(t, pool, clk, pid, "tool-t", model.DispatchStatusSuccess, 10)
	clk.Advance(2 * time.Hour)
	insertDispatch(t, pool, clk, pid, "tool-t", model.DispatchStatusSuccess, 10)

	svc := NewInsightsService(pool, clk)
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
