package repo

import (
	"database/sql"
	"testing"

	"be/internal/model"
)

// TestResetSingleAgentSession_HappyPath verifies that a completed session is reset to
// callback status, findings cleared, and ended_at set.
func TestResetSingleAgentSession_HappyPath(t *testing.T) {
	t.Parallel()
	database, repo, wfiID := setupTestDB(t)
	defer database.Close()

	session := &model.AgentSession{
		ID:                 "single-1",
		ProjectID:          "proj",
		TicketID:           "TKT-1",
		WorkflowInstanceID: wfiID,
		Phase:              "analyzer",
		AgentType:          "analyzer",
		Status:             model.AgentSessionCompleted,
		Findings:           sql.NullString{String: `{"key":"val"}`, Valid: true},
	}
	if err := repo.Create(session); err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	if err := repo.ResetSingleAgentSession(wfiID, "analyzer"); err != nil {
		t.Fatalf("ResetSingleAgentSession failed: %v", err)
	}

	got, err := repo.Get("single-1")
	if err != nil {
		t.Fatalf("failed to get session: %v", err)
	}
	if got.Status != model.AgentSessionCallback {
		t.Errorf("status = %q, want callback", got.Status)
	}
	if got.Findings.String != "{}" {
		t.Errorf("findings = %q, want {}", got.Findings.String)
	}
	if !got.EndedAt.Valid {
		t.Error("ended_at should be set")
	}
}

// TestResetSingleAgentSession_ExcludesRunning verifies running sessions are not reset.
func TestResetSingleAgentSession_ExcludesRunning(t *testing.T) {
	t.Parallel()
	database, repo, wfiID := setupTestDB(t)
	defer database.Close()

	running := &model.AgentSession{
		ID:                 "single-running",
		ProjectID:          "proj",
		TicketID:           "TKT-2",
		WorkflowInstanceID: wfiID,
		Phase:              "builder",
		AgentType:          "builder",
		Status:             model.AgentSessionRunning,
		Findings:           sql.NullString{String: `{"x":"y"}`, Valid: true},
	}
	if err := repo.Create(running); err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	if err := repo.ResetSingleAgentSession(wfiID, "builder"); err != nil {
		t.Fatalf("ResetSingleAgentSession failed: %v", err)
	}

	got, _ := repo.Get("single-running")
	if got.Status != model.AgentSessionRunning {
		t.Errorf("running session status = %q, want running", got.Status)
	}
	if got.Findings.String != `{"x":"y"}` {
		t.Errorf("running session findings changed unexpectedly: %s", got.Findings.String)
	}
}

// TestResetSingleAgentSession_ExcludesContinued verifies continued sessions are not reset.
func TestResetSingleAgentSession_ExcludesContinued(t *testing.T) {
	t.Parallel()
	database, repo, wfiID := setupTestDB(t)
	defer database.Close()

	continued := &model.AgentSession{
		ID:                 "single-continued",
		ProjectID:          "proj",
		TicketID:           "TKT-3",
		WorkflowInstanceID: wfiID,
		Phase:              "verifier",
		AgentType:          "verifier",
		Status:             model.AgentSessionContinued,
		Findings:           sql.NullString{String: `{"a":"b"}`, Valid: true},
	}
	if err := repo.Create(continued); err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	if err := repo.ResetSingleAgentSession(wfiID, "verifier"); err != nil {
		t.Fatalf("ResetSingleAgentSession failed: %v", err)
	}

	got, _ := repo.Get("single-continued")
	if got.Status != model.AgentSessionContinued {
		t.Errorf("continued session status = %q, want continued", got.Status)
	}
}

// TestResetSingleAgentSession_NoMatchIsNoOp verifies no error when phase has no sessions.
func TestResetSingleAgentSession_NoMatchIsNoOp(t *testing.T) {
	t.Parallel()
	database, repo, wfiID := setupTestDB(t)
	defer database.Close()

	if err := repo.ResetSingleAgentSession(wfiID, "nonexistent-phase"); err != nil {
		t.Errorf("expected no error for unmatched phase, got: %v", err)
	}
}

// TestResetAgentSessionsInWorkflow_HappyPath verifies multiple phases are reset in one call.
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
			Findings:           sql.NullString{String: `{"phase":"analyzer"}`, Valid: true},
		},
		{
			ID:                 "multi-2",
			ProjectID:          "proj",
			TicketID:           "TKT-4",
			WorkflowInstanceID: wfiID,
			Phase:              "builder",
			AgentType:          "builder",
			Status:             model.AgentSessionFailed,
			Findings:           sql.NullString{String: `{"phase":"builder"}`, Valid: true},
		},
		{
			ID:                 "multi-3",
			ProjectID:          "proj",
			TicketID:           "TKT-4",
			WorkflowInstanceID: wfiID,
			Phase:              "verifier",
			AgentType:          "verifier",
			Status:             model.AgentSessionCompleted,
			Findings:           sql.NullString{String: `{"phase":"verifier"}`, Valid: true},
		},
	}
	for _, s := range sessions {
		if err := repo.Create(s); err != nil {
			t.Fatalf("failed to create session %s: %v", s.ID, err)
		}
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
		if got.Findings.String != "{}" {
			t.Errorf("%s: findings = %q, want {}", id, got.Findings.String)
		}
		if !got.EndedAt.Valid {
			t.Errorf("%s: ended_at should be set", id)
		}
	}

	// verifier not in phases list — should be untouched
	got3, _ := repo.Get("multi-3")
	if got3.Status != model.AgentSessionCompleted {
		t.Errorf("multi-3: status = %q, want completed", got3.Status)
	}
}

// TestResetAgentSessionsInWorkflow_ExcludesRunningAndContinued verifies active sessions are skipped.
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
			Findings:           sql.NullString{String: `{"r":"1"}`, Valid: true},
		},
		{
			ID:                 "multi-cont",
			ProjectID:          "proj",
			TicketID:           "TKT-5",
			WorkflowInstanceID: wfiID,
			Phase:              "analyzer",
			AgentType:          "analyzer",
			Status:             model.AgentSessionContinued,
			Findings:           sql.NullString{String: `{"c":"2"}`, Valid: true},
		},
	}
	for _, s := range sessions {
		if err := repo.Create(s); err != nil {
			t.Fatalf("failed to create session: %v", err)
		}
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
		Findings:           sql.NullString{String: `{"data":"test"}`, Valid: true},
	}
	if err := repo.Create(session); err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	if err := repo.ResetAgentSessionsInWorkflow(wfiID, []string{}); err != nil {
		t.Fatalf("expected no error for empty phases, got: %v", err)
	}

	got, _ := repo.Get("multi-empty")
	if got.Status != model.AgentSessionCompleted {
		t.Errorf("status = %q, want completed", got.Status)
	}
	if got.Findings.String != `{"data":"test"}` {
		t.Errorf("findings changed unexpectedly: %s", got.Findings.String)
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
			Findings:           sql.NullString{String: `{"p":"analyzer"}`, Valid: true},
		},
		{
			ID:                 "multi-other",
			ProjectID:          "proj",
			TicketID:           "TKT-7",
			WorkflowInstanceID: wfiID,
			Phase:              "builder",
			AgentType:          "builder",
			Status:             model.AgentSessionCompleted,
			Findings:           sql.NullString{String: `{"p":"builder"}`, Valid: true},
		},
	}
	for _, s := range sessions {
		if err := repo.Create(s); err != nil {
			t.Fatalf("failed to create session: %v", err)
		}
	}

	if err := repo.ResetAgentSessionsInWorkflow(wfiID, []string{"analyzer"}); err != nil {
		t.Fatalf("ResetAgentSessionsInWorkflow failed: %v", err)
	}

	target, _ := repo.Get("multi-target")
	if target.Status != model.AgentSessionCallback {
		t.Errorf("analyzer session status = %q, want callback", target.Status)
	}

	other, _ := repo.Get("multi-other")
	if other.Status != model.AgentSessionCompleted {
		t.Errorf("builder session status = %q, want completed (should be untouched)", other.Status)
	}
	if other.Findings.String != `{"p":"builder"}` {
		t.Errorf("builder session findings changed unexpectedly: %s", other.Findings.String)
	}
}
