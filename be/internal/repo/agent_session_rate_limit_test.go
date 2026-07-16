package repo

import (
	"database/sql"
	"testing"
	"time"

	"be/internal/db"
	"be/internal/model"
)

// makeRateLimitSession returns an AgentSession with all three rate-limit fields populated.
func makeRateLimitSession(id, wfiID string) *model.AgentSession {
	return &model.AgentSession{
		ID:                  id,
		ProjectID:           "proj",
		TicketID:            "TKT-1",
		WorkflowInstanceID:  wfiID,
		Phase:               "phase0",
		AgentType:           "test-agent",
		ModelID:             sql.NullString{String: "sonnet-5", Valid: true},
		Status:              model.AgentSessionRunning,
		RateLimitRetryCount: 3,
		RateLimitUntilTs:    sql.NullString{String: "2026-05-25T12:00:00Z", Valid: true},
		LastRetryClass:      sql.NullString{String: "rate_limit", Valid: true},
	}
}

// TestAgentSession_RateLimitFieldsRoundTrip verifies Create + Get persists all three new columns.
func TestAgentSession_RateLimitFieldsRoundTrip(t *testing.T) {
	t.Parallel()
	_, r, wfiID := setupConfigTestDB(t)

	sess := makeRateLimitSession("sess-rl-1", wfiID)
	if err := r.Create(sess); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := r.Get("sess-rl-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if got.RateLimitRetryCount != 3 {
		t.Errorf("RateLimitRetryCount = %d, want 3", got.RateLimitRetryCount)
	}
	wantTs := "2026-05-25T12:00:00Z"
	if !got.RateLimitUntilTs.Valid || got.RateLimitUntilTs.String != wantTs {
		t.Errorf("RateLimitUntilTs = {Valid:%v, String:%q}, want {true, %q}",
			got.RateLimitUntilTs.Valid, got.RateLimitUntilTs.String, wantTs)
	}
	if !got.LastRetryClass.Valid || got.LastRetryClass.String != "rate_limit" {
		t.Errorf("LastRetryClass = {Valid:%v, String:%q}, want {true, rate_limit}",
			got.LastRetryClass.Valid, got.LastRetryClass.String)
	}
}

// TestAgentSession_RateLimitFieldsDefault verifies that a session created without rate-limit
// fields has zero/null values on readback.
func TestAgentSession_RateLimitFieldsDefault(t *testing.T) {
	t.Parallel()
	_, r, wfiID := setupConfigTestDB(t)

	sess := makeSession("sess-rl-default", wfiID, "")
	if err := r.Create(sess); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := r.Get("sess-rl-default")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if got.RateLimitRetryCount != 0 {
		t.Errorf("RateLimitRetryCount = %d, want 0", got.RateLimitRetryCount)
	}
	if got.RateLimitUntilTs.Valid {
		t.Errorf("RateLimitUntilTs.Valid = true, want false (null)")
	}
	if got.LastRetryClass.Valid {
		t.Errorf("LastRetryClass.Valid = true, want false (null)")
	}
}

// TestListLiveByProject_RateLimitUntilTs_Present verifies the field is scanned when set.
func TestListLiveByProject_RateLimitUntilTs_Present(t *testing.T) {
	t.Parallel()
	f := setupLogFixture(t)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	ts := "2026-05-25T12:00:00Z"

	if _, err := f.db.Exec(`
		INSERT INTO agent_sessions
		(id, project_id, ticket_id, workflow_instance_id, phase, agent_type,
		 status, pid, started_at, rate_limit_until_ts, created_at, updated_at)
		VALUES (?, 'log-proj', '', ?, 'ph', 'ag', 'running', ?, ?, ?, ?, ?)`,
		"rl-ts-sess", f.wfiID, int64(9001), now, ts, now, now,
	); err != nil {
		t.Fatalf("insert: %v", err)
	}

	rows, err := f.repo.ListLiveByProject("log-proj")
	if err != nil {
		t.Fatalf("ListLiveByProject: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}
	if !rows[0].RateLimitUntilTs.Valid {
		t.Fatalf("RateLimitUntilTs.Valid = false, want true")
	}
	if rows[0].RateLimitUntilTs.String != ts {
		t.Errorf("RateLimitUntilTs.String = %q, want %q", rows[0].RateLimitUntilTs.String, ts)
	}
}

// TestListLiveByProject_RateLimitUntilTs_Null verifies the field is null when not set.
func TestListLiveByProject_RateLimitUntilTs_Null(t *testing.T) {
	t.Parallel()
	f := setupLogFixture(t)
	base := time.Now().UTC()

	insertLiveSession(t, f, "rl-null-sess", "log-proj", f.wfiID, "running", int64(9002), base)

	rows, err := f.repo.ListLiveByProject("log-proj")
	if err != nil {
		t.Fatalf("ListLiveByProject: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}
	if rows[0].RateLimitUntilTs.Valid {
		t.Errorf("RateLimitUntilTs.Valid = true, want false (null)")
	}
}

// insertContinuedSessionWithRateLimit inserts a continued session with rate_limit_until_ts set.
// Pass rateLimitUntilTs="" to store NULL (excluded by GetRunning WHERE clause).
func insertContinuedSessionWithRateLimit(t *testing.T, database *db.DB, id, wfiID, rateLimitUntilTs string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	var rlTs interface{}
	if rateLimitUntilTs != "" {
		rlTs = rateLimitUntilTs
	}
	_, err := database.Exec(`
		INSERT INTO agent_sessions
		(id, project_id, ticket_id, workflow_instance_id, phase, agent_type, model_id, status, started_at, created_at, updated_at, rate_limit_until_ts)
		VALUES (?, 'proj', 'TKT-1', ?, 'test-phase', 'test-agent', 'sonnet-5', 'continued', ?, ?, ?, ?)`,
		id, wfiID, now, now, now, rlTs)
	if err != nil {
		t.Fatalf("insertContinuedSessionWithRateLimit(%s): %v", id, err)
	}
}

// TestGetRunning_ContinuedWithRateLimitReturned verifies that continued sessions with a
// non-NULL rate_limit_until_ts (both future and past) are returned by GetRunning.
// Filtering by future/past is the caller's responsibility.
func TestGetRunning_ContinuedWithRateLimitReturned(t *testing.T) {
	t.Parallel()
	database, r, wfiID := setupRunningTestDB(t)
	defer database.Close()

	futureTs := time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano)
	pastTs := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339Nano)
	insertContinuedSessionWithRateLimit(t, database, "sess-future", wfiID, futureTs)
	insertContinuedSessionWithRateLimit(t, database, "sess-past", wfiID, pastTs)

	sessions, err := r.GetRunning(50)
	if err != nil {
		t.Fatalf("GetRunning() error: %v", err)
	}
	if len(sessions) != 2 {
		t.Errorf("GetRunning() = %d sessions, want 2 (caller owns future/past filter)", len(sessions))
	}
	for _, s := range sessions {
		if s.Status != model.AgentSessionContinued {
			t.Errorf("session %s status = %s, want continued", s.ID, s.Status)
		}
		if !s.RateLimitUntilTs.Valid {
			t.Errorf("session %s RateLimitUntilTs must be valid (non-NULL)", s.ID)
		}
	}
}

// TestGetRunning_ContinuedWithNullRateLimitExcluded verifies that continued sessions
// with NULL rate_limit_until_ts are excluded by the GetRunning WHERE clause.
func TestGetRunning_ContinuedWithNullRateLimitExcluded(t *testing.T) {
	t.Parallel()
	database, r, wfiID := setupRunningTestDB(t)
	defer database.Close()

	insertContinuedSessionWithRateLimit(t, database, "sess-null-rl", wfiID, "")

	sessions, err := r.GetRunning(50)
	if err != nil {
		t.Fatalf("GetRunning() error: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("GetRunning() = %d sessions, want 0 (continued+null rate_limit excluded)", len(sessions))
	}
}

// TestGetRunning_MixedRunningAndContinuedRateLimit verifies running and
// continued+non-null rate_limit sessions are both returned.
func TestGetRunning_MixedRunningAndContinuedRateLimit(t *testing.T) {
	t.Parallel()
	database, r, wfiID := setupRunningTestDB(t)
	defer database.Close()

	now := time.Now().UTC()
	insertRunningSession(t, database, "sess-running", wfiID, model.AgentSessionRunning, now)
	futureTs := now.Add(time.Hour).Format(time.RFC3339Nano)
	insertContinuedSessionWithRateLimit(t, database, "sess-rl", wfiID, futureTs)

	sessions, err := r.GetRunning(50)
	if err != nil {
		t.Fatalf("GetRunning() error: %v", err)
	}
	if len(sessions) != 2 {
		t.Errorf("GetRunning() = %d sessions, want 2 (running + continued+rate_limit)", len(sessions))
	}
	counts := make(map[model.AgentSessionStatus]int)
	for _, s := range sessions {
		counts[s.Status]++
	}
	if counts[model.AgentSessionRunning] != 1 {
		t.Errorf("running count = %d, want 1", counts[model.AgentSessionRunning])
	}
	if counts[model.AgentSessionContinued] != 1 {
		t.Errorf("continued count = %d, want 1", counts[model.AgentSessionContinued])
	}
}
