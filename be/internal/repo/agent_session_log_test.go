package repo

import (
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
)

func mustExecLog(t *testing.T, d *db.DB, q string, args ...interface{}) {
	t.Helper()
	if _, err := d.Exec(q, args...); err != nil {
		t.Fatalf("mustExecLog %q: %v", q, err)
	}
}

type logFixture struct {
	db             *db.DB
	repo           *AgentSessionRepo
	wfiID          string
	wfiScheduledID string
	wfiBID         string
}

func setupLogFixture(t *testing.T) *logFixture {
	t.Helper()
	database := newTestDB(t)

	mustExecLog(t, database, `INSERT INTO projects (id, name, created_at, updated_at) VALUES ('log-proj', 'P', datetime('now'), datetime('now'))`)
	mustExecLog(t, database, `INSERT INTO projects (id, name, created_at, updated_at) VALUES ('log-proj-b', 'PB', datetime('now'), datetime('now'))`)
	mustExecLog(t, database, `INSERT INTO workflows (project_id, id, description, scope_type, created_at, updated_at) VALUES ('log-proj', 'log-wf', '', 'project', datetime('now'), datetime('now'))`)
	mustExecLog(t, database, `INSERT INTO workflows (project_id, id, description, scope_type, created_at, updated_at) VALUES ('log-proj-b', 'log-wf-b', '', 'project', datetime('now'), datetime('now'))`)
	mustExecLog(t, database, `INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, scope_type, findings, created_at, updated_at) VALUES ('log-wfi', 'log-proj', '', 'log-wf', 'active', 'project', '{}', datetime('now'), datetime('now'))`)
	mustExecLog(t, database, `INSERT INTO scheduled_tasks (id, project_id, name, cron_expression, created_at, updated_at) VALUES ('log-sched', 'log-proj', 'S', '0 * * * *', datetime('now'), datetime('now'))`)
	mustExecLog(t, database, `INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, scope_type, findings, scheduled_task_id, created_at, updated_at) VALUES ('log-wfi-s', 'log-proj', '', 'log-wf', 'completed', 'project', '{}', 'log-sched', datetime('now'), datetime('now'))`)
	mustExecLog(t, database, `INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, scope_type, findings, created_at, updated_at) VALUES ('log-wfi-b', 'log-proj-b', '', 'log-wf-b', 'active', 'project', '{}', datetime('now'), datetime('now'))`)

	return &logFixture{
		db:             database,
		repo:           NewAgentSessionRepo(database, clock.Real()),
		wfiID:          "log-wfi",
		wfiScheduledID: "log-wfi-s",
		wfiBID:         "log-wfi-b",
	}
}

func insertLogSess(t *testing.T, d *db.DB, id, projectID, wfiID, status string, endedAt time.Time) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	started := endedAt.Add(-5 * time.Second).UTC().Format(time.RFC3339Nano)
	ended := endedAt.UTC().Format(time.RFC3339Nano)
	if _, err := d.Exec(`
		INSERT INTO agent_sessions
		(id, project_id, ticket_id, workflow_instance_id, phase, agent_type, status, started_at, ended_at, created_at, updated_at)
		VALUES (?, ?, '', ?, 'ph', 'ag', ?, ?, ?, ?, ?)`,
		id, projectID, wfiID, status, started, ended, now, now,
	); err != nil {
		t.Fatalf("insertLogSess(%s): %v", id, err)
	}
}

func TestListFinished_ExcludesActiveStatuses(t *testing.T) {
	t.Parallel()
	f := setupLogFixture(t)
	base := time.Now().UTC()

	excluded := []string{"running", "continued", "callback", "user_interactive"}
	for i, s := range excluded {
		insertLogSess(t, f.db, "ex-"+s, "log-proj", f.wfiID, s, base.Add(time.Duration(i)*time.Second))
	}

	included := []string{"completed", "failed", "timeout", "skipped", "project_completed", "interactive_completed"}
	for i, s := range included {
		insertLogSess(t, f.db, "in-"+s, "log-proj", f.wfiID, s, base.Add(time.Duration(len(excluded)+i)*time.Second))
	}

	rows, total, err := f.repo.ListFinished(ListFinishedFilter{ProjectID: "log-proj"}, 1, 100)
	if err != nil {
		t.Fatalf("ListFinished: %v", err)
	}
	if total != len(included) {
		t.Errorf("total = %d, want %d", total, len(included))
	}
	if len(rows) != len(included) {
		t.Fatalf("rows count = %d, want %d", len(rows), len(included))
	}
	for _, row := range rows {
		for _, ex := range excluded {
			if string(row.Status) == ex {
				t.Errorf("excluded status %q returned in results", ex)
			}
		}
	}
}

func TestListFinished_OrdersByEndedAtDesc(t *testing.T) {
	t.Parallel()
	f := setupLogFixture(t)
	base := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)

	insertLogSess(t, f.db, "ord-b", "log-proj", f.wfiID, "completed", base.Add(2*time.Second))
	insertLogSess(t, f.db, "ord-c", "log-proj", f.wfiID, "completed", base.Add(3*time.Second))
	insertLogSess(t, f.db, "ord-a", "log-proj", f.wfiID, "completed", base.Add(1*time.Second))

	rows, _, err := f.repo.ListFinished(ListFinishedFilter{ProjectID: "log-proj"}, 1, 10)
	if err != nil {
		t.Fatalf("ListFinished: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("rows count = %d, want 3", len(rows))
	}
	wantOrder := []string{"ord-c", "ord-b", "ord-a"}
	for i, want := range wantOrder {
		if rows[i].SessionID != want {
			t.Errorf("rows[%d].SessionID = %q, want %q", i, rows[i].SessionID, want)
		}
	}
}

func TestListFinished_ProjectScoping(t *testing.T) {
	t.Parallel()
	f := setupLogFixture(t)
	base := time.Now().UTC()

	insertLogSess(t, f.db, "scope-a", "log-proj", f.wfiID, "completed", base)
	insertLogSess(t, f.db, "scope-b", "log-proj-b", f.wfiBID, "completed", base)

	rows, total, err := f.repo.ListFinished(ListFinishedFilter{ProjectID: "log-proj"}, 1, 100)
	if err != nil {
		t.Fatalf("ListFinished: %v", err)
	}
	if total != 1 {
		t.Errorf("total = %d, want 1 (proj-a only)", total)
	}
	if len(rows) != 1 {
		t.Fatalf("rows count = %d, want 1", len(rows))
	}
	if rows[0].SessionID != "scope-a" {
		t.Errorf("SessionID = %q, want scope-a", rows[0].SessionID)
	}
	if rows[0].ProjectID != "log-proj" {
		t.Errorf("ProjectID = %q, want log-proj", rows[0].ProjectID)
	}
}

func TestListFinished_ScheduledTaskID(t *testing.T) {
	t.Parallel()
	f := setupLogFixture(t)
	base := time.Now().UTC()

	insertLogSess(t, f.db, "sched-no", "log-proj", f.wfiID, "completed", base)
	insertLogSess(t, f.db, "sched-yes", "log-proj", f.wfiScheduledID, "completed", base.Add(time.Second))

	rows, _, err := f.repo.ListFinished(ListFinishedFilter{ProjectID: "log-proj"}, 1, 100)
	if err != nil {
		t.Fatalf("ListFinished: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows count = %d, want 2", len(rows))
	}
	// DESC ordering: sched-yes (base+1s) first, sched-no (base) second.
	if rows[0].SessionID != "sched-yes" {
		t.Fatalf("rows[0].SessionID = %q, want sched-yes", rows[0].SessionID)
	}
	if !rows[0].ScheduledTaskID.Valid {
		t.Errorf("rows[0].ScheduledTaskID.Valid = false, want true")
	}
	if rows[0].ScheduledTaskID.String != "log-sched" {
		t.Errorf("rows[0].ScheduledTaskID.String = %q, want log-sched", rows[0].ScheduledTaskID.String)
	}
	if rows[1].SessionID != "sched-no" {
		t.Fatalf("rows[1].SessionID = %q, want sched-no", rows[1].SessionID)
	}
	if rows[1].ScheduledTaskID.Valid {
		t.Errorf("rows[1].ScheduledTaskID.Valid = true, want false (no scheduled task)")
	}
	if rows[0].WorkflowID != "log-wf" {
		t.Errorf("rows[0].WorkflowID = %q, want log-wf", rows[0].WorkflowID)
	}
}

func TestListFinished_Pagination(t *testing.T) {
	t.Parallel()
	f := setupLogFixture(t)
	base := time.Date(2025, 7, 1, 0, 0, 0, 0, time.UTC)

	// pag-1 gets base+1s … pag-5 gets base+5s (newest).
	for i, id := range []string{"pag-1", "pag-2", "pag-3", "pag-4", "pag-5"} {
		insertLogSess(t, f.db, id, "log-proj", f.wfiID, "completed", base.Add(time.Duration(i+1)*time.Second))
	}

	_, total, err := f.repo.ListFinished(ListFinishedFilter{ProjectID: "log-proj"}, 1, 100)
	if err != nil {
		t.Fatalf("ListFinished count: %v", err)
	}
	if total != 5 {
		t.Errorf("total = %d, want 5", total)
	}

	// Order DESC: pag-5, pag-4, pag-3, pag-2, pag-1.
	// Page 2 with per_page=2 → offset=2 → pag-3, pag-2.
	page2, _, err := f.repo.ListFinished(ListFinishedFilter{ProjectID: "log-proj"}, 2, 2)
	if err != nil {
		t.Fatalf("ListFinished page2: %v", err)
	}
	if len(page2) != 2 {
		t.Fatalf("page2 count = %d, want 2", len(page2))
	}
	if page2[0].SessionID != "pag-3" {
		t.Errorf("page2[0].SessionID = %q, want pag-3", page2[0].SessionID)
	}
	if page2[1].SessionID != "pag-2" {
		t.Errorf("page2[1].SessionID = %q, want pag-2", page2[1].SessionID)
	}
}

func TestListFinished_Empty(t *testing.T) {
	t.Parallel()
	f := setupLogFixture(t)

	rows, total, err := f.repo.ListFinished(ListFinishedFilter{ProjectID: "log-proj"}, 1, 20)
	if err != nil {
		t.Fatalf("ListFinished: %v", err)
	}
	if total != 0 {
		t.Errorf("total = %d, want 0", total)
	}
	if rows != nil {
		t.Errorf("rows = %v, want nil", rows)
	}
}
