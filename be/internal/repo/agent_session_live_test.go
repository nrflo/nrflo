package repo

import (
	"testing"
	"time"
)

// insertLiveSession inserts a running session with a pid for ListLiveByProject tests.
func insertLiveSession(t *testing.T, f *logFixture, id, projectID, wfiID, status string, pid interface{}, startedAt time.Time) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	started := startedAt.UTC().Format(time.RFC3339Nano)
	if _, err := f.db.Exec(`
		INSERT INTO agent_sessions
		(id, project_id, ticket_id, workflow_instance_id, phase, agent_type, status, pid, started_at, created_at, updated_at)
		VALUES (?, ?, '', ?, 'ph', 'ag', ?, ?, ?, ?, ?)`,
		id, projectID, wfiID, status, pid, started, now, now,
	); err != nil {
		t.Fatalf("insertLiveSession(%s): %v", id, err)
	}
}

func TestListLiveByProject_RunningIncluded(t *testing.T) {
	t.Parallel()
	f := setupLogFixture(t)
	base := time.Now().UTC()

	insertLiveSession(t, f, "live-run", "log-proj", f.wfiID, "running", int64(9999), base)

	rows, err := f.repo.ListLiveByProject("log-proj")
	if err != nil {
		t.Fatalf("ListLiveByProject: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}
	if rows[0].SessionID != "live-run" {
		t.Errorf("SessionID = %q, want live-run", rows[0].SessionID)
	}
	if !rows[0].PID.Valid || rows[0].PID.Int64 != 9999 {
		t.Errorf("PID = {Valid:%v, Int64:%d}, want {true, 9999}", rows[0].PID.Valid, rows[0].PID.Int64)
	}
}

func TestListLiveByProject_UserInteractiveIncluded(t *testing.T) {
	t.Parallel()
	f := setupLogFixture(t)
	base := time.Now().UTC()

	insertLiveSession(t, f, "live-ui", "log-proj", f.wfiID, "user_interactive", int64(8888), base)

	rows, err := f.repo.ListLiveByProject("log-proj")
	if err != nil {
		t.Fatalf("ListLiveByProject: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}
	if rows[0].SessionID != "live-ui" {
		t.Errorf("SessionID = %q, want live-ui", rows[0].SessionID)
	}
}

func TestListLiveByProject_TerminalStatusExcluded(t *testing.T) {
	t.Parallel()
	f := setupLogFixture(t)
	base := time.Now().UTC()

	terminal := []string{"completed", "failed", "timeout", "skipped", "project_completed",
		"interactive_completed", "continued", "callback"}
	for i, s := range terminal {
		insertLiveSession(t, f, "term-"+s, "log-proj", f.wfiID, s,
			int64(1000+i), base.Add(time.Duration(i)*time.Second))
	}

	rows, err := f.repo.ListLiveByProject("log-proj")
	if err != nil {
		t.Fatalf("ListLiveByProject: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("len(rows) = %d, want 0 (all terminal)", len(rows))
	}
}

func TestListLiveByProject_NullPidExcluded(t *testing.T) {
	t.Parallel()
	f := setupLogFixture(t)
	base := time.Now().UTC()

	insertLiveSession(t, f, "null-pid", "log-proj", f.wfiID, "running", nil, base)

	rows, err := f.repo.ListLiveByProject("log-proj")
	if err != nil {
		t.Fatalf("ListLiveByProject: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("len(rows) = %d, want 0 (null pid excluded)", len(rows))
	}
}

func TestListLiveByProject_ZeroPidExcluded(t *testing.T) {
	t.Parallel()
	f := setupLogFixture(t)
	base := time.Now().UTC()

	insertLiveSession(t, f, "zero-pid", "log-proj", f.wfiID, "running", int64(0), base)

	rows, err := f.repo.ListLiveByProject("log-proj")
	if err != nil {
		t.Fatalf("ListLiveByProject: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("len(rows) = %d, want 0 (zero pid excluded)", len(rows))
	}
}

func TestListLiveByProject_CrossProjectExcluded(t *testing.T) {
	t.Parallel()
	f := setupLogFixture(t)
	base := time.Now().UTC()

	insertLiveSession(t, f, "other-proj-sess", "log-proj-b", f.wfiBID, "running", int64(7777), base)
	insertLiveSession(t, f, "this-proj-sess", "log-proj", f.wfiID, "running", int64(6666), base.Add(time.Second))

	rows, err := f.repo.ListLiveByProject("log-proj")
	if err != nil {
		t.Fatalf("ListLiveByProject: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1 (own project only)", len(rows))
	}
	if rows[0].SessionID != "this-proj-sess" {
		t.Errorf("SessionID = %q, want this-proj-sess", rows[0].SessionID)
	}
}

func TestListLiveByProject_OrderedByStartedAtDesc(t *testing.T) {
	t.Parallel()
	f := setupLogFixture(t)
	base := time.Date(2025, 8, 1, 0, 0, 0, 0, time.UTC)

	insertLiveSession(t, f, "live-a", "log-proj", f.wfiID, "running", int64(1001), base.Add(1*time.Second))
	insertLiveSession(t, f, "live-c", "log-proj", f.wfiID, "running", int64(1003), base.Add(3*time.Second))
	insertLiveSession(t, f, "live-b", "log-proj", f.wfiID, "running", int64(1002), base.Add(2*time.Second))

	rows, err := f.repo.ListLiveByProject("log-proj")
	if err != nil {
		t.Fatalf("ListLiveByProject: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("len(rows) = %d, want 3", len(rows))
	}
	want := []string{"live-c", "live-b", "live-a"}
	for i, w := range want {
		if rows[i].SessionID != w {
			t.Errorf("rows[%d].SessionID = %q, want %q", i, rows[i].SessionID, w)
		}
	}
}

func TestListLiveByProject_Empty(t *testing.T) {
	t.Parallel()
	f := setupLogFixture(t)

	rows, err := f.repo.ListLiveByProject("log-proj")
	if err != nil {
		t.Fatalf("ListLiveByProject: %v", err)
	}
	if rows != nil {
		t.Errorf("rows = %v, want nil", rows)
	}
}

func TestListLiveByProject_WorkflowIDJoined(t *testing.T) {
	t.Parallel()
	f := setupLogFixture(t)
	base := time.Now().UTC()

	insertLiveSession(t, f, "live-wfid", "log-proj", f.wfiID, "running", int64(5555), base)

	rows, err := f.repo.ListLiveByProject("log-proj")
	if err != nil {
		t.Fatalf("ListLiveByProject: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}
	if rows[0].WorkflowID != "log-wf" {
		t.Errorf("WorkflowID = %q, want log-wf", rows[0].WorkflowID)
	}
	if rows[0].WorkflowInstanceID != f.wfiID {
		t.Errorf("WorkflowInstanceID = %q, want %q", rows[0].WorkflowInstanceID, f.wfiID)
	}
}

func TestListLiveByProject_ProjectIDField(t *testing.T) {
	t.Parallel()
	f := setupLogFixture(t)
	base := time.Now().UTC()

	insertLiveSession(t, f, "live-projid", "log-proj", f.wfiID, "running", int64(4444), base)

	rows, err := f.repo.ListLiveByProject("log-proj")
	if err != nil {
		t.Fatalf("ListLiveByProject: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}
	if rows[0].ProjectID != "log-proj" {
		t.Errorf("ProjectID = %q, want log-proj", rows[0].ProjectID)
	}
}

func TestListLiveByProject_BothStatuses(t *testing.T) {
	t.Parallel()
	f := setupLogFixture(t)
	base := time.Now().UTC()

	insertLiveSession(t, f, "both-running", "log-proj", f.wfiID, "running", int64(3001), base)
	insertLiveSession(t, f, "both-ui", "log-proj", f.wfiID, "user_interactive", int64(3002), base.Add(time.Second))

	rows, err := f.repo.ListLiveByProject("log-proj")
	if err != nil {
		t.Fatalf("ListLiveByProject: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("len(rows) = %d, want 2 (running + user_interactive)", len(rows))
	}
}
