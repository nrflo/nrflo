package repo

import (
	"testing"
	"time"

	"be/internal/model"
)

// Console rows carry ticket_id='' and a NULL workflow_instance_id, exactly like
// project-scoped workflow agents. These queries do not JOIN workflow_instances,
// so only the kind filter keeps console rows out of user-facing listings.

func TestGetByProjectScope_ExcludesConsoleSessions(t *testing.T) {
	t.Parallel()
	database, r := setupConsoleTestDB(t)
	now := time.Now().UTC()

	insertConsoleSession(t, database, "console-scope", "tok-scope", model.AgentSessionUserInteractive, now)

	sessions, err := r.GetByProjectScope("proj", "")
	if err != nil {
		t.Fatalf("GetByProjectScope: %v", err)
	}
	for _, s := range sessions {
		if s.ID == "console-scope" {
			t.Fatalf("GetByProjectScope returned console row %q, want excluded", s.ID)
		}
	}
}

func TestListFinished_TotalExcludesClosedConsoleSessions(t *testing.T) {
	t.Parallel()
	database, r := setupConsoleTestDB(t)
	now := time.Now().UTC()

	// A closed console row: status interactive_completed is not in the excluded
	// active-status list, and it has no workflow_instance_id to JOIN on — so an
	// unfiltered COUNT would report a total the row query can never return.
	insertConsoleSession(t, database, "console-fin", "tok-fin", model.AgentSessionInteractiveCompleted, now)

	rows, total, err := r.ListFinished(ListFinishedFilter{ProjectID: "proj"}, 1, 100)
	if err != nil {
		t.Fatalf("ListFinished: %v", err)
	}
	if total != 0 {
		t.Errorf("total = %d, want 0 (closed console row must not be counted)", total)
	}
	if len(rows) != 0 {
		t.Errorf("rows = %d, want 0", len(rows))
	}
}

func TestListFinished_TotalMatchesRowsWithConsoleAndWorkflowAgent(t *testing.T) {
	t.Parallel()
	database, r := setupConsoleTestDB(t)
	now := time.Now().UTC()

	insertWorkflowAgentSession(t, database, "wf-done", model.AgentSessionCompleted, now)
	insertConsoleSession(t, database, "console-done", "tok-done", model.AgentSessionInteractiveCompleted, now)

	rows, total, err := r.ListFinished(ListFinishedFilter{ProjectID: "proj"}, 1, 100)
	if err != nil {
		t.Fatalf("ListFinished: %v", err)
	}
	if total != len(rows) {
		t.Fatalf("total = %d but returned %d rows; count and row query disagree", total, len(rows))
	}
	if total != 1 {
		t.Fatalf("total = %d, want 1 (only the workflow agent)", total)
	}
	if rows[0].SessionID != "wf-done" {
		t.Errorf("row SessionID = %q, want wf-done", rows[0].SessionID)
	}
}
