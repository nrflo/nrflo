package repo

import (
	"encoding/json"
	"testing"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
)

// seedSessionFinding stores a session-scoped finding for sessionID, as emit_findings does.
func seedSessionFinding(t *testing.T, database *db.DB, sessionID, wfiID, key string) {
	t.Helper()
	if err := NewFindingRepo(database, clock.Real()).Upsert(
		"session", sessionID, key, json.RawMessage(`"stale"`),
		Denorm{ProjectID: "proj", WorkflowInstanceID: wfiID},
		Actor{ID: sessionID, Source: "agent"}); err != nil {
		t.Fatalf("seed finding for %s: %v", sessionID, err)
	}
}

// countSessionFindings returns the number of session-scoped findings for sessionID.
func countSessionFindings(t *testing.T, database *db.DB, sessionID string) int {
	t.Helper()
	var n int
	if err := database.QueryRow(
		`SELECT COUNT(*) FROM findings WHERE scope='session' AND scope_id=?`, sessionID).Scan(&n); err != nil {
		t.Fatalf("count findings for %s: %v", sessionID, err)
	}
	return n
}

// TestResetAgentSessionsInWorkflow_HappyPath verifies multiple phases are reset in one call
// and their session findings are deleted, while unspecified phases keep theirs.
func TestResetAgentSessionsInWorkflow_HappyPath(t *testing.T) {
	t.Parallel()
	database, repo, wfiID := setupTestDB(t)
	defer database.Close()

	sessions := []*model.AgentSession{
		{
			ID:                 "multi-1",
			ProjectID:          "proj",
			TicketID:           "TKT-4",
			WorkflowInstanceID: wfiID,
			Phase:              "analyzer",
			AgentType:          "analyzer",
			Status:             model.AgentSessionCompleted,
		},
		{
			ID:                 "multi-2",
			ProjectID:          "proj",
			TicketID:           "TKT-4",
			WorkflowInstanceID: wfiID,
			Phase:              "builder",
			AgentType:          "builder",
			Status:             model.AgentSessionFailed,
		},
		{
			ID:                 "multi-3",
			ProjectID:          "proj",
			TicketID:           "TKT-4",
			WorkflowInstanceID: wfiID,
			Phase:              "verifier",
			AgentType:          "verifier",
			Status:             model.AgentSessionCompleted,
		},
	}
	for _, s := range sessions {
		if err := repo.Create(s); err != nil {
			t.Fatalf("failed to create session %s: %v", s.ID, err)
		}
		seedSessionFinding(t, database, s.ID, wfiID, "result")
	}

	if err := repo.ResetAgentSessionsInWorkflow(wfiID, []string{"analyzer", "builder"}); err != nil {
		t.Fatalf("ResetAgentSessionsInWorkflow failed: %v", err)
	}

	for _, id := range []string{"multi-1", "multi-2"} {
		got, err := repo.Get(id)
		if err != nil {
			t.Fatalf("failed to get %s: %v", id, err)
		}
		if got.Status != model.AgentSessionCallback {
			t.Errorf("%s: status = %q, want callback", id, got.Status)
		}
		if !got.EndedAt.Valid {
			t.Errorf("%s: ended_at should be set", id)
		}
		if n := countSessionFindings(t, database, id); n != 0 {
			t.Errorf("%s: findings = %d, want 0 (reset must delete them)", id, n)
		}
		// The delete must be audited like FindingRepo.DeleteKeys.
		var hist int
		if err := database.QueryRow(
			`SELECT COUNT(*) FROM findings_history WHERE scope='session' AND scope_id=? AND operation='delete'`,
			id).Scan(&hist); err != nil {
			t.Fatalf("%s: count history: %v", id, err)
		}
		if hist != 1 {
			t.Errorf("%s: delete history rows = %d, want 1", id, hist)
		}
	}

	// verifier not in phases list — session and findings should be untouched
	got3, _ := repo.Get("multi-3")
	if got3.Status != model.AgentSessionCompleted {
		t.Errorf("multi-3: status = %q, want completed", got3.Status)
	}
	if n := countSessionFindings(t, database, "multi-3"); n != 1 {
		t.Errorf("multi-3: findings = %d, want 1 (unspecified phase must keep them)", n)
	}
}

// TestResetAgentSessionsInWorkflow_ExcludesRunningAndContinued verifies active sessions
// are skipped: neither their status nor their findings change.
func TestResetAgentSessionsInWorkflow_ExcludesRunningAndContinued(t *testing.T) {
	t.Parallel()
	database, repo, wfiID := setupTestDB(t)
	defer database.Close()

	sessions := []*model.AgentSession{
		{
			ID:                 "multi-run",
			ProjectID:          "proj",
			TicketID:           "TKT-5",
			WorkflowInstanceID: wfiID,
			Phase:              "analyzer",
			AgentType:          "analyzer",
			Status:             model.AgentSessionRunning,
		},
		{
			ID:                 "multi-cont",
			ProjectID:          "proj",
			TicketID:           "TKT-5",
			WorkflowInstanceID: wfiID,
			Phase:              "analyzer",
			AgentType:          "analyzer",
			Status:             model.AgentSessionContinued,
		},
	}
	for _, s := range sessions {
		if err := repo.Create(s); err != nil {
			t.Fatalf("failed to create session: %v", err)
		}
		seedSessionFinding(t, database, s.ID, wfiID, "result")
	}

	if err := repo.ResetAgentSessionsInWorkflow(wfiID, []string{"analyzer"}); err != nil {
		t.Fatalf("ResetAgentSessionsInWorkflow failed: %v", err)
	}

	run, _ := repo.Get("multi-run")
	if run.Status != model.AgentSessionRunning {
		t.Errorf("running session status = %q, want running", run.Status)
	}
	cont, _ := repo.Get("multi-cont")
	if cont.Status != model.AgentSessionContinued {
		t.Errorf("continued session status = %q, want continued", cont.Status)
	}
	for _, id := range []string{"multi-run", "multi-cont"} {
		if n := countSessionFindings(t, database, id); n != 1 {
			t.Errorf("%s: findings = %d, want 1 (active sessions must keep them)", id, n)
		}
	}
}

// TestResetAgentSessionsInWorkflow_EmptyPhasesIsNoOp verifies empty list causes no changes.
func TestResetAgentSessionsInWorkflow_EmptyPhasesIsNoOp(t *testing.T) {
	t.Parallel()
	database, repo, wfiID := setupTestDB(t)
	defer database.Close()

	session := &model.AgentSession{
		ID:                 "multi-empty",
		ProjectID:          "proj",
		TicketID:           "TKT-6",
		WorkflowInstanceID: wfiID,
		Phase:              "analyzer",
		AgentType:          "analyzer",
		Status:             model.AgentSessionCompleted,
	}
	if err := repo.Create(session); err != nil {
		t.Fatalf("failed to create session: %v", err)
	}
	seedSessionFinding(t, database, session.ID, wfiID, "result")

	if err := repo.ResetAgentSessionsInWorkflow(wfiID, []string{}); err != nil {
		t.Fatalf("expected no error for empty phases, got: %v", err)
	}

	got, _ := repo.Get("multi-empty")
	if got.Status != model.AgentSessionCompleted {
		t.Errorf("status = %q, want completed", got.Status)
	}
	if n := countSessionFindings(t, database, "multi-empty"); n != 1 {
		t.Errorf("findings = %d, want 1 (no-op must not delete)", n)
	}
}

// TestResetAgentSessionsInWorkflow_OnlySpecifiedPhases verifies unspecified phases are untouched.
func TestResetAgentSessionsInWorkflow_OnlySpecifiedPhases(t *testing.T) {
	t.Parallel()
	database, repo, wfiID := setupTestDB(t)
	defer database.Close()

	sessions := []*model.AgentSession{
		{
			ID:                 "multi-target",
			ProjectID:          "proj",
			TicketID:           "TKT-7",
			WorkflowInstanceID: wfiID,
			Phase:              "analyzer",
			AgentType:          "analyzer",
			Status:             model.AgentSessionCompleted,
		},
		{
			ID:                 "multi-other",
			ProjectID:          "proj",
			TicketID:           "TKT-7",
			WorkflowInstanceID: wfiID,
			Phase:              "builder",
			AgentType:          "builder",
			Status:             model.AgentSessionCompleted,
		},
	}
	for _, s := range sessions {
		if err := repo.Create(s); err != nil {
			t.Fatalf("failed to create session: %v", err)
		}
		seedSessionFinding(t, database, s.ID, wfiID, "result")
	}

	if err := repo.ResetAgentSessionsInWorkflow(wfiID, []string{"analyzer"}); err != nil {
		t.Fatalf("ResetAgentSessionsInWorkflow failed: %v", err)
	}

	target, _ := repo.Get("multi-target")
	if target.Status != model.AgentSessionCallback {
		t.Errorf("analyzer session status = %q, want callback", target.Status)
	}
	if n := countSessionFindings(t, database, "multi-target"); n != 0 {
		t.Errorf("multi-target: findings = %d, want 0", n)
	}

	other, _ := repo.Get("multi-other")
	if other.Status != model.AgentSessionCompleted {
		t.Errorf("builder session status = %q, want completed (should be untouched)", other.Status)
	}
	if n := countSessionFindings(t, database, "multi-other"); n != 1 {
		t.Errorf("multi-other: findings = %d, want 1", n)
	}
}
