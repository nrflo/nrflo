package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
)

// newRunningAgentsServerWithClock creates a Server using the given clock for time control.
func newRunningAgentsServerWithClock(t *testing.T, clk clock.Clock) (*Server, *db.DB) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "rl_handler_test.db")
	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := apiCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	t.Cleanup(func() {
		pool.Close()
		database.Close()
	})
	s := &Server{dataPath: dbPath, pool: pool, clock: clk}
	return s, database
}

// insertContinuedHandlerSession inserts a continued agent_session with rate_limit_until_ts.
// Pass rateLimitUntilTs="" to store NULL. retryCount sets rate_limit_retry_count.
func insertContinuedHandlerSession(t *testing.T, database *db.DB, id, wfiID, projectID, rateLimitUntilTs string, retryCount int) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	var rlTs interface{}
	if rateLimitUntilTs != "" {
		rlTs = rateLimitUntilTs
	}
	_, err := database.Exec(`
		INSERT INTO agent_sessions
		(id, project_id, ticket_id, workflow_instance_id, phase, agent_type, model_id,
		 status, started_at, created_at, updated_at, rate_limit_until_ts, rate_limit_retry_count)
		VALUES (?, ?, 'TKT-1', ?, 'impl', 'implementor', 'sonnet', 'continued', ?, ?, ?, ?, ?)`,
		id, projectID, wfiID, now, now, now,
		sql.NullString{String: rateLimitUntilTs, Valid: rateLimitUntilTs != ""},
		retryCount)
	if err != nil {
		t.Fatalf("insertContinuedHandlerSession(%s): %v", id, err)
	}
	_ = rlTs
}

// TestHandleGetRunningAgents_ContinuedFutureRateLimitIncluded verifies that a continued
// session whose rate_limit_until_ts is in the future appears in the response with
// waiting_for_rate_limit=true and the rate-limit fields set.
func TestHandleGetRunningAgents_ContinuedFutureRateLimitIncluded(t *testing.T) {
	fixedNow := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	s, database := newRunningAgentsServerWithClock(t, clock.NewTest(fixedNow))

	wfiID := seedProject(t, database, "proj-rl-future", "RL Future Project")
	futureTs := fixedNow.Add(time.Hour).Format(time.RFC3339Nano)
	insertContinuedHandlerSession(t, database, "sess-rl-future", wfiID, "proj-rl-future", futureTs, 2)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/running", nil)
	rr := httptest.NewRecorder()
	s.handleGetRunningAgents(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	agents, ok := resp["agents"].([]interface{})
	if !ok || len(agents) != 1 {
		t.Fatalf("agents = %v, want 1 element", resp["agents"])
	}
	agent := agents[0].(map[string]interface{})
	if agent["waiting_for_rate_limit"] != true {
		t.Errorf("waiting_for_rate_limit = %v, want true", agent["waiting_for_rate_limit"])
	}
	if agent["rate_limit_until_ts"] != futureTs {
		t.Errorf("rate_limit_until_ts = %v, want %q", agent["rate_limit_until_ts"], futureTs)
	}
	if agent["rate_limit_retry_count"].(float64) != 2 {
		t.Errorf("rate_limit_retry_count = %v, want 2", agent["rate_limit_retry_count"])
	}
	if resp["count"].(float64) != 1 {
		t.Errorf("count = %v, want 1", resp["count"])
	}
}

// TestHandleGetRunningAgents_ContinuedPastRateLimitExcluded verifies that a continued
// session whose rate_limit_until_ts has already passed is excluded from the response.
func TestHandleGetRunningAgents_ContinuedPastRateLimitExcluded(t *testing.T) {
	fixedNow := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	s, database := newRunningAgentsServerWithClock(t, clock.NewTest(fixedNow))

	wfiID := seedProject(t, database, "proj-rl-past", "RL Past Project")
	pastTs := fixedNow.Add(-time.Hour).Format(time.RFC3339Nano)
	insertContinuedHandlerSession(t, database, "sess-rl-past", wfiID, "proj-rl-past", pastTs, 1)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/running", nil)
	rr := httptest.NewRecorder()
	s.handleGetRunningAgents(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	var resp map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&resp)
	agents := resp["agents"].([]interface{})
	if len(agents) != 0 {
		t.Errorf("agents = %d, want 0 (expired rate limit excluded)", len(agents))
	}
	if resp["count"].(float64) != 0 {
		t.Errorf("count = %v, want 0", resp["count"])
	}
}

// TestHandleGetRunningAgents_RunningHasNoWaitingField verifies that a normal running
// session does not include waiting_for_rate_limit in the response.
func TestHandleGetRunningAgents_RunningHasNoWaitingField(t *testing.T) {
	s, database := newRunningAgentsServer(t)
	defer database.Close()

	wfiID := seedProject(t, database, "proj-run-no-rl", "Running No RL")
	startedAt := time.Now().UTC().Format(time.RFC3339Nano)
	insertHandlerSession(t, database, "sess-no-rl", wfiID, "proj-run-no-rl", "running", startedAt)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/running", nil)
	rr := httptest.NewRecorder()
	s.handleGetRunningAgents(rr, req)

	var resp map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&resp)
	agents := resp["agents"].([]interface{})
	if len(agents) != 1 {
		t.Fatalf("agents = %d, want 1", len(agents))
	}
	agent := agents[0].(map[string]interface{})
	if _, exists := agent["waiting_for_rate_limit"]; exists {
		t.Errorf("running session must not have waiting_for_rate_limit field")
	}
	if _, exists := agent["rate_limit_until_ts"]; exists {
		t.Errorf("running session must not have rate_limit_until_ts field")
	}
}

// TestHandleGetRunningAgents_CountReflectsPostFilterSize verifies that count matches
// the number of agents after filtering out expired continued sessions.
func TestHandleGetRunningAgents_CountReflectsPostFilterSize(t *testing.T) {
	fixedNow := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	s, database := newRunningAgentsServerWithClock(t, clock.NewTest(fixedNow))

	wfiID := seedProject(t, database, "proj-count-rl", "Count RL Project")
	futureTs := fixedNow.Add(time.Hour).Format(time.RFC3339Nano)
	pastTs := fixedNow.Add(-time.Hour).Format(time.RFC3339Nano)
	startedAt := fixedNow.Add(-10 * time.Minute).Format(time.RFC3339Nano)
	// 3 sessions: running (kept) + future rl (kept) + past rl (filtered)
	insertHandlerSession(t, database, "sess-run", wfiID, "proj-count-rl", "running", startedAt)
	insertContinuedHandlerSession(t, database, "sess-future-rl", wfiID, "proj-count-rl", futureTs, 1)
	insertContinuedHandlerSession(t, database, "sess-past-rl", wfiID, "proj-count-rl", pastTs, 1)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/running", nil)
	rr := httptest.NewRecorder()
	s.handleGetRunningAgents(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	var resp map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&resp)
	agents := resp["agents"].([]interface{})
	if len(agents) != 2 {
		t.Errorf("agents = %d, want 2 (running + future-rl; past-rl filtered)", len(agents))
	}
	if resp["count"].(float64) != 2 {
		t.Errorf("count = %v, want 2", resp["count"])
	}
}
