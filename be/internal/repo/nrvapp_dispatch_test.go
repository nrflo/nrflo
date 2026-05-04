package repo

import (
	"path/filepath"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
)

func setupNrvappDispatchDBWithClock(t *testing.T, clk clock.Clock) (*NrvappDispatchRepo, string) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	_, err = database.Exec(
		`INSERT INTO projects (id, name, created_at, updated_at) VALUES ('proj-1', 'Test', datetime('now'), datetime('now'))`)
	if err != nil {
		t.Fatalf("insert project: %v", err)
	}
	return NewNrvappDispatchRepo(database, clk), "proj-1"
}

func setupNrvappDispatchDB(t *testing.T) (*NrvappDispatchRepo, string) {
	t.Helper()
	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	return setupNrvappDispatchDBWithClock(t, clk)
}

func setupNrvappDispatchAndReviewDB(t *testing.T) (*NrvappDispatchRepo, *NrvappReviewRepo, string) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	_, err = database.Exec(
		`INSERT INTO projects (id, name, created_at, updated_at) VALUES ('proj-1', 'Test', datetime('now'), datetime('now'))`)
	if err != nil {
		t.Fatalf("insert project: %v", err)
	}
	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	return NewNrvappDispatchRepo(database, clk), NewNrvappReviewRepo(database, clk), "proj-1"
}

func makeDispatch(projectID, toolName, status string, durationMs int64) *model.NrvappToolDispatch {
	return &model.NrvappToolDispatch{
		ProjectID:  projectID,
		ToolName:   toolName,
		Input:      `{}`,
		Status:     status,
		DurationMs: durationMs,
	}
}

func TestNrvappDispatchRepo_InsertSetsIDAndTimestamp(t *testing.T) {
	r, projectID := setupNrvappDispatchDB(t)

	d := makeDispatch(projectID, "write_file", model.DispatchStatusSuccess, 100)
	if err := r.Insert(d); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if d.ID == "" {
		t.Errorf("ID not set after Insert")
	}
	if d.CreatedAt.IsZero() {
		t.Errorf("CreatedAt is zero")
	}
}

func TestNrvappDispatchRepo_InsertErrorStatus(t *testing.T) {
	r, projectID := setupNrvappDispatchDB(t)

	errMsg := "connection refused"
	d := &model.NrvappToolDispatch{
		ProjectID:  projectID,
		ToolName:   "exec_cmd",
		Input:      `{}`,
		Status:     model.DispatchStatusError,
		ErrorMsg:   &errMsg,
		DurationMs: 50,
	}
	if err := r.Insert(d); err != nil {
		t.Fatalf("Insert error-status dispatch: %v", err)
	}
	if d.ID == "" {
		t.Errorf("ID not set after Insert")
	}
}

func TestNrvappDispatchRepo_ListSummary_Counts(t *testing.T) {
	r, projectID := setupNrvappDispatchDB(t)
	since := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).Add(-time.Second)

	for i := 0; i < 3; i++ {
		r.Insert(makeDispatch(projectID, "tool", model.DispatchStatusSuccess, 10))
	}
	for i := 0; i < 2; i++ {
		r.Insert(makeDispatch(projectID, "tool", model.DispatchStatusError, 10))
	}

	summary, err := r.ListSummary(projectID, since)
	if err != nil {
		t.Fatalf("ListSummary: %v", err)
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

func TestNrvappDispatchRepo_ListSummary_Percentiles(t *testing.T) {
	r, projectID := setupNrvappDispatchDB(t)
	since := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).Add(-time.Second)

	// 10 dispatches with durations 10..100ms (sorted ascending by query)
	for i := 1; i <= 10; i++ {
		r.Insert(makeDispatch(projectID, "tool", model.DispatchStatusSuccess, int64(i*10)))
	}

	summary, err := r.ListSummary(projectID, since)
	if err != nil {
		t.Fatalf("ListSummary: %v", err)
	}
	// p50Index(10) = 10*50/100 = 5 → sorted[5] = 60ms (0-indexed: 10,20,30,40,50,60,...)
	if summary.P50Ms != 60 {
		t.Errorf("P50Ms = %d, want 60", summary.P50Ms)
	}
	// p95Index(10) = 10*95/100 = 9 → sorted[9] = 100ms
	if summary.P95Ms != 100 {
		t.Errorf("P95Ms = %d, want 100", summary.P95Ms)
	}
}

func TestNrvappDispatchRepo_ListSummary_Empty(t *testing.T) {
	r, projectID := setupNrvappDispatchDB(t)
	since := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).Add(-time.Second)

	summary, err := r.ListSummary(projectID, since)
	if err != nil {
		t.Fatalf("ListSummary empty: %v", err)
	}
	if summary.Total != 0 || summary.Success != 0 || summary.Error != 0 {
		t.Errorf("empty: Total=%d Success=%d Error=%d, want all 0", summary.Total, summary.Success, summary.Error)
	}
	if summary.P50Ms != 0 || summary.P95Ms != 0 {
		t.Errorf("empty: P50Ms=%d P95Ms=%d, want 0", summary.P50Ms, summary.P95Ms)
	}
}

func TestNrvappDispatchRepo_ListSummary_SinceFilters(t *testing.T) {
	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	r, projectID := setupNrvappDispatchDBWithClock(t, clk)

	// Insert 2 records at T0
	r.Insert(makeDispatch(projectID, "t", model.DispatchStatusSuccess, 10))
	r.Insert(makeDispatch(projectID, "t", model.DispatchStatusSuccess, 10))

	// Insert 1 more at T0+1h
	clk.Advance(time.Hour)
	r.Insert(makeDispatch(projectID, "t", model.DispatchStatusSuccess, 10))

	// since = T0+30m: only the last record is included
	since := time.Date(2026, 1, 1, 0, 30, 0, 0, time.UTC)
	summary, err := r.ListSummary(projectID, since)
	if err != nil {
		t.Fatalf("ListSummary with since: %v", err)
	}
	if summary.Total != 1 {
		t.Errorf("Total = %d, want 1 (since filter)", summary.Total)
	}
}

func TestNrvappDispatchRepo_EditRateByTool(t *testing.T) {
	dispatchRepo, reviewRepo, projectID := setupNrvappDispatchAndReviewDB(t)
	since := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).Add(-time.Second)

	sess1 := "sess-write"
	sess2 := "sess-exec"
	sess3 := "sess-read"

	// write_file: dispatch + approved review with no draft → approve_no_edits
	d1 := &model.NrvappToolDispatch{ProjectID: projectID, SessionID: &sess1, ToolName: "write_file", Input: `{}`, Status: model.DispatchStatusSuccess, DurationMs: 10}
	dispatchRepo.Insert(d1)
	rev1 := &model.NrvappReviewItem{ProjectID: projectID, SessionID: &sess1, ToolName: "write_file", Input: `{}`}
	reviewRepo.Insert(rev1)
	reviewRepo.Approve(rev1.ID, projectID) // draft=NULL → approve_no_edits

	// exec_cmd: dispatch + approved review with draft != input → approve_with_edits
	d2 := &model.NrvappToolDispatch{ProjectID: projectID, SessionID: &sess2, ToolName: "exec_cmd", Input: `{}`, Status: model.DispatchStatusSuccess, DurationMs: 20}
	dispatchRepo.Insert(d2)
	rev2 := &model.NrvappReviewItem{ProjectID: projectID, SessionID: &sess2, ToolName: "exec_cmd", Input: `{}`}
	reviewRepo.Insert(rev2)
	reviewRepo.UpdateDraft(rev2.ID, projectID, "edited") // "edited" != "{}"
	reviewRepo.Approve(rev2.ID, projectID)

	// read_file: dispatch with no matching review → all zeros
	d3 := &model.NrvappToolDispatch{ProjectID: projectID, SessionID: &sess3, ToolName: "read_file", Input: `{}`, Status: model.DispatchStatusSuccess, DurationMs: 5}
	dispatchRepo.Insert(d3)

	rows, err := dispatchRepo.EditRateByTool(projectID, since)
	if err != nil {
		t.Fatalf("EditRateByTool: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("EditRateByTool row count = %d, want 3", len(rows))
	}

	byTool := map[string]*model.EditRateRow{}
	for _, row := range rows {
		byTool[row.ToolName] = row
	}

	if wf := byTool["write_file"]; wf == nil {
		t.Error("write_file row missing")
	} else if wf.ApproveNoEdits != 1 || wf.ApproveWithEdits != 0 || wf.Rejected != 0 {
		t.Errorf("write_file = {%d,%d,%d}, want {1,0,0}", wf.ApproveNoEdits, wf.ApproveWithEdits, wf.Rejected)
	}

	if ec := byTool["exec_cmd"]; ec == nil {
		t.Error("exec_cmd row missing")
	} else if ec.ApproveNoEdits != 0 || ec.ApproveWithEdits != 1 || ec.Rejected != 0 {
		t.Errorf("exec_cmd = {%d,%d,%d}, want {0,1,0}", ec.ApproveNoEdits, ec.ApproveWithEdits, ec.Rejected)
	}

	if rf := byTool["read_file"]; rf == nil {
		t.Error("read_file row missing")
	} else if rf.ApproveNoEdits != 0 || rf.ApproveWithEdits != 0 || rf.Rejected != 0 {
		t.Errorf("read_file = {%d,%d,%d}, want {0,0,0} (no review)", rf.ApproveNoEdits, rf.ApproveWithEdits, rf.Rejected)
	}
}

func TestNrvappDispatchRepo_EditRateByTool_Empty(t *testing.T) {
	r, _, projectID := setupNrvappDispatchAndReviewDB(t)
	since := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).Add(-time.Second)

	rows, err := r.EditRateByTool(projectID, since)
	if err != nil {
		t.Fatalf("EditRateByTool empty: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("EditRateByTool empty: count = %d, want 0", len(rows))
	}
}

func TestNrvappDispatchRepo_Throughput_Buckets(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	clk := clock.NewTest(t0)
	r, projectID := setupNrvappDispatchDBWithClock(t, clk)
	since := t0.Add(-time.Second)

	// T0: 2 dispatches → bucket 0 (minute 0)
	r.Insert(makeDispatch(projectID, "t", model.DispatchStatusSuccess, 10))
	r.Insert(makeDispatch(projectID, "t", model.DispatchStatusSuccess, 10))
	// T0+30s: still bucket 0 (same 60s window)
	clk.Advance(30 * time.Second)
	r.Insert(makeDispatch(projectID, "t", model.DispatchStatusSuccess, 10))
	// T0+60s: bucket 1
	clk.Advance(30 * time.Second)
	r.Insert(makeDispatch(projectID, "t", model.DispatchStatusSuccess, 10))
	// T0+120s: bucket 2
	clk.Advance(60 * time.Second)
	r.Insert(makeDispatch(projectID, "t", model.DispatchStatusSuccess, 10))

	points, err := r.Throughput(projectID, since, 60)
	if err != nil {
		t.Fatalf("Throughput: %v", err)
	}
	if len(points) != 3 {
		t.Fatalf("Throughput bucket count = %d, want 3", len(points))
	}
	if points[0].Count != 3 {
		t.Errorf("bucket[0].Count = %d, want 3 (T0 + T0+30s)", points[0].Count)
	}
	if points[1].Count != 1 {
		t.Errorf("bucket[1].Count = %d, want 1 (T0+60s)", points[1].Count)
	}
	if points[2].Count != 1 {
		t.Errorf("bucket[2].Count = %d, want 1 (T0+120s)", points[2].Count)
	}
	if !points[1].BucketStart.After(points[0].BucketStart) {
		t.Errorf("bucket[1].BucketStart %v not after bucket[0].BucketStart %v", points[1].BucketStart, points[0].BucketStart)
	}
}

func TestNrvappDispatchRepo_Throughput_Empty(t *testing.T) {
	r, projectID := setupNrvappDispatchDB(t)
	// since is in the future → no records match
	since := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).Add(time.Hour)

	points, err := r.Throughput(projectID, since, 60)
	if err != nil {
		t.Fatalf("Throughput empty: %v", err)
	}
	if len(points) != 0 {
		t.Errorf("Throughput empty: count = %d, want 0", len(points))
	}
}
