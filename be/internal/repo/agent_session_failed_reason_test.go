package repo

import (
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/model"
)

// TestUpdateStatusToFailedWithReason_HappyPath verifies status, result, result_reason,
// and ended_at are all persisted correctly.
func TestUpdateStatusToFailedWithReason_HappyPath(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		reason string
	}{
		{"user_killed", "user_killed"},
		{"user_aborted", "user_aborted"},
		{"custom reason with spaces", "custom reason with spaces"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			database, r, wfiID := setupTestDB(t)
			defer database.Close()

			sess := &model.AgentSession{
				ID:                 "sess-fail-" + tc.name,
				ProjectID:          "proj",
				TicketID:           "TKT-FR-1",
				WorkflowInstanceID: wfiID,
				Phase:              "phase-a",
				AgentType:          "analyzer",
				Status:             model.AgentSessionUserInteractive,
			}
			if err := r.Create(sess); err != nil {
				t.Fatalf("Create: %v", err)
			}

			if err := r.UpdateStatusToFailedWithReason(sess.ID, tc.reason); err != nil {
				t.Fatalf("UpdateStatusToFailedWithReason(%q): %v", tc.reason, err)
			}

			got, err := r.Get(sess.ID)
			if err != nil {
				t.Fatalf("Get: %v", err)
			}
			if got.Status != model.AgentSessionFailed {
				t.Errorf("status = %q, want %q", got.Status, model.AgentSessionFailed)
			}
			if !got.Result.Valid || got.Result.String != "fail" {
				t.Errorf("result = %v, want {Valid:true, String:\"fail\"}", got.Result)
			}
			if !got.ResultReason.Valid || got.ResultReason.String != tc.reason {
				t.Errorf("result_reason = %v, want %q", got.ResultReason, tc.reason)
			}
			if !got.EndedAt.Valid || got.EndedAt.String == "" {
				t.Error("ended_at should be set (non-empty)")
			}
			if _, parseErr := time.Parse(time.RFC3339Nano, got.EndedAt.String); parseErr != nil {
				t.Errorf("ended_at %q does not parse as RFC3339Nano: %v", got.EndedAt.String, parseErr)
			}
		})
	}
}

// TestUpdateStatusToFailedWithReason_NotFound verifies not-found error for unknown ID.
func TestUpdateStatusToFailedWithReason_NotFound(t *testing.T) {
	t.Parallel()
	database, r, _ := setupTestDB(t)
	defer database.Close()

	err := r.UpdateStatusToFailedWithReason("nonexistent-session-xyz", "user_killed")
	if err == nil {
		t.Fatal("expected error for nonexistent session, got nil")
	}
}

// TestUpdateStatusToFailedWithReason_TimestampUsesRepoClock verifies the injected clock
// is used for ended_at and updated_at.
func TestUpdateStatusToFailedWithReason_TimestampUsesRepoClock(t *testing.T) {
	t.Parallel()
	database, _, wfiID := setupTestDB(t)
	defer database.Close()

	fixedTime := time.Date(2025, 8, 20, 12, 0, 0, 0, time.UTC)
	clk := clock.NewTest(fixedTime)
	r := NewAgentSessionRepo(database, clk)

	sess := &model.AgentSession{
		ID:                 "sess-clk-fail",
		ProjectID:          "proj",
		TicketID:           "TKT-FR-CLK",
		WorkflowInstanceID: wfiID,
		Phase:              "phase-clk",
		AgentType:          "analyzer",
		Status:             model.AgentSessionUserInteractive,
	}
	if err := r.Create(sess); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := r.UpdateStatusToFailedWithReason(sess.ID, "user_killed"); err != nil {
		t.Fatalf("UpdateStatusToFailedWithReason: %v", err)
	}

	got, err := r.Get(sess.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !got.EndedAt.Valid {
		t.Fatal("ended_at should be set")
	}
	parsed, err := time.Parse(time.RFC3339Nano, got.EndedAt.String)
	if err != nil {
		t.Fatalf("ended_at %q not parseable: %v", got.EndedAt.String, err)
	}
	if !parsed.Equal(fixedTime) {
		t.Errorf("ended_at = %v, want %v", parsed, fixedTime)
	}
}

// TestUpdateStatusToFailedWithReason_ReasonRoundTrips verifies the reason parameter
// is stored verbatim and not hardcoded inside the function.
func TestUpdateStatusToFailedWithReason_ReasonRoundTrips(t *testing.T) {
	t.Parallel()
	database, r, wfiID := setupTestDB(t)
	defer database.Close()

	reasons := []string{"user_killed", "user_aborted", "timeout_expired"}

	for i, reason := range reasons {
		sess := &model.AgentSession{
			ID:                 "sess-rr-" + string(rune('a'+i)),
			ProjectID:          "proj",
			TicketID:           "TKT-RR",
			WorkflowInstanceID: wfiID,
			Phase:              "phase",
			AgentType:          "agent",
			Status:             model.AgentSessionRunning,
		}
		if err := r.Create(sess); err != nil {
			t.Fatalf("Create[%d]: %v", i, err)
		}
		if err := r.UpdateStatusToFailedWithReason(sess.ID, reason); err != nil {
			t.Fatalf("UpdateStatusToFailedWithReason[%d]: %v", i, err)
		}
		got, err := r.Get(sess.ID)
		if err != nil {
			t.Fatalf("Get[%d]: %v", i, err)
		}
		if got.ResultReason.String != reason {
			t.Errorf("[%d] result_reason = %q, want %q", i, got.ResultReason.String, reason)
		}
	}
}
