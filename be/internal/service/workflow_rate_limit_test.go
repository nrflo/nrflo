package service

import (
	"path/filepath"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
)

// setupDeriveTestEnvWithClock creates the same DB structure as setupDeriveTestEnv
// but builds the WorkflowService with the given clock for deterministic time comparisons.
func setupDeriveTestEnvWithClock(t *testing.T, clk clock.Clock) (*db.Pool, *WorkflowService, string) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "derive_clk.db")
	if err := svcCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err = pool.Exec(`INSERT INTO projects (id, name, root_path, created_at, updated_at) VALUES ('test-proj', 'Test', '/tmp', ?, ?)`, now, now); err != nil {
		t.Fatalf("project: %v", err)
	}
	if _, err = pool.Exec(`INSERT INTO workflows (id, project_id, description, scope_type, created_at, updated_at) VALUES ('test-wf', 'test-proj', '', 'ticket', ?, ?)`, now, now); err != nil {
		t.Fatalf("workflow: %v", err)
	}
	for _, ad := range []struct {
		id    string
		layer int
	}{{"analyzer", 0}, {"builder", 1}} {
		if _, err = pool.Exec(
			`INSERT INTO agent_definitions (id, project_id, workflow_id, prompt, layer, created_at, updated_at) VALUES (?, 'test-proj', 'test-wf', '', ?, ?, ?)`,
			ad.id, ad.layer, now, now); err != nil {
			t.Fatalf("agent_def %s: %v", ad.id, err)
		}
	}
	wfiID := "wfi-test"
	if _, err = pool.Exec(
		`INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, scope_type, status, retry_count, created_at, updated_at) VALUES (?, 'test-proj', '', 'test-wf', 'ticket', 'active', 0, ?, ?)`,
		wfiID, now, now); err != nil {
		t.Fatalf("wfi: %v", err)
	}
	return pool, NewWorkflowService(pool, clk), wfiID
}

// insertSessionRateLimit inserts a continued agent_session with rate_limit_until_ts set.
// Pass rateLimitUntilTs="" to store NULL. createdAt="" uses current time.
func insertSessionRateLimit(t *testing.T, pool *db.Pool, id, wfiID, agentType, rateLimitUntilTs string, retryCount int, createdAt string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if createdAt == "" {
		createdAt = now
	}
	var rlTs interface{}
	if rateLimitUntilTs != "" {
		rlTs = rateLimitUntilTs
	}
	_, err := pool.Exec(`
		INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type,
			status, restart_count, started_at, created_at, updated_at,
			rate_limit_until_ts, rate_limit_retry_count)
		VALUES (?, 'test-proj', '', ?, ?, ?, 'continued', 0, ?, ?, ?, ?, ?)`,
		id, wfiID, agentType, agentType, createdAt, createdAt, now, rlTs, retryCount)
	if err != nil {
		t.Fatalf("insertSessionRateLimit(%s): %v", id, err)
	}
}

// TestDerivePhaseStatuses_RateLimit verifies that continued sessions with a future
// rate_limit_until_ts become "rate_limited" and those with a past ts remain "pending".
func TestDerivePhaseStatuses_RateLimit(t *testing.T) {
	t.Parallel()
	fixedNow := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	futureTs := fixedNow.Add(time.Hour).Format(time.RFC3339Nano)
	pastTs := fixedNow.Add(-time.Hour).Format(time.RFC3339Nano)

	tests := []struct {
		name       string
		ts         string
		retryCount int
		wantStatus string
	}{
		{"future_ts_rate_limited", futureTs, 3, "rate_limited"},
		{"past_ts_pending", pastTs, 1, "pending"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pool, svc, wfiID := setupDeriveTestEnvWithClock(t, clock.NewTest(fixedNow))
			insertSessionRateLimit(t, pool, "s1", wfiID, "analyzer", tc.ts, tc.retryCount, "")

			got := svc.derivePhaseStatuses(wfiID, twoPhases)
			ps := got["analyzer"]
			if ps.Status != tc.wantStatus {
				t.Errorf("analyzer status = %q, want %q", ps.Status, tc.wantStatus)
			}
			if tc.wantStatus == "rate_limited" {
				if ps.RateLimitUntilTs != tc.ts {
					t.Errorf("RateLimitUntilTs = %q, want %q", ps.RateLimitUntilTs, tc.ts)
				}
				if ps.RateLimitRetryCount != tc.retryCount {
					t.Errorf("RateLimitRetryCount = %d, want %d", ps.RateLimitRetryCount, tc.retryCount)
				}
			}
		})
	}
}

// TestDerivePhaseStatuses_RateLimitedAddedToSeenBlocksMainLoop verifies that the pre-pass
// adds the agent_type to seen so the main loop does not process it again.
func TestDerivePhaseStatuses_RateLimitedAddedToSeenBlocksMainLoop(t *testing.T) {
	t.Parallel()
	fixedNow := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	futureTs := fixedNow.Add(time.Hour).Format(time.RFC3339Nano)

	pool, svc, wfiID := setupDeriveTestEnvWithClock(t, clock.NewTest(fixedNow))
	// Older completed session for the same agent_type
	insertSession(t, pool, "s-old", wfiID, "analyzer", "completed", "pass", "2025-01-01T00:00:00Z")
	// Newer continued+future row — pre-pass should win
	insertSessionRateLimit(t, pool, "s-new", wfiID, "analyzer", futureTs, 1, "2025-06-01T11:00:00Z")

	got := svc.derivePhaseStatuses(wfiID, twoPhases)
	ps := got["analyzer"]
	if ps.Status != "rate_limited" {
		t.Errorf("analyzer status = %q, want rate_limited (continued+future is the newest activity)", ps.Status)
	}
}

// TestDerivePhaseStatuses_RateLimitMaxLayerForSkipInference verifies that a rate-limited
// phase updates maxLayer so earlier-layer phases without sessions are inferred as skipped.
func TestDerivePhaseStatuses_RateLimitMaxLayerForSkipInference(t *testing.T) {
	t.Parallel()
	fixedNow := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	futureTs := fixedNow.Add(time.Hour).Format(time.RFC3339Nano)

	pool, svc, wfiID := setupDeriveTestEnvWithClock(t, clock.NewTest(fixedNow))
	// builder (layer 1) is rate-limited; analyzer (layer 0) has no session
	insertSessionRateLimit(t, pool, "s1", wfiID, "builder", futureTs, 1, "")

	got := svc.derivePhaseStatuses(wfiID, twoPhases)
	if got["builder"].Status != "rate_limited" {
		t.Errorf("builder status = %q, want rate_limited", got["builder"].Status)
	}
	if got["analyzer"].Status != "skipped" {
		t.Errorf("analyzer status = %q, want skipped (lower layer inferred skipped by maxLayer)", got["analyzer"].Status)
	}
}

// TestBuildActiveAgentsMap_RateLimit verifies that continued+future sessions appear in the
// map with rate-limit fields set, while continued+past sessions are filtered out.
func TestBuildActiveAgentsMap_RateLimit(t *testing.T) {
	t.Parallel()
	fixedNow := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	futureTs := fixedNow.Add(time.Hour).Format(time.RFC3339Nano)
	pastTs := fixedNow.Add(-time.Hour).Format(time.RFC3339Nano)

	tests := []struct {
		name        string
		ts          string
		retryCount  int
		wantInMap   bool
		wantWaiting bool
	}{
		{"future_included", futureTs, 2, true, true},
		{"past_excluded", pastTs, 1, false, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pool, svc, wfiID := setupDeriveTestEnvWithClock(t, clock.NewTest(fixedNow))
			insertSessionRateLimit(t, pool, "s1", wfiID, "rl-agent", tc.ts, tc.retryCount, "")

			result := svc.buildActiveAgentsMap(wfiID, map[string][]RestartDetail{})
			_, inMap := result["rl-agent"]
			if inMap != tc.wantInMap {
				t.Errorf("key in map = %v, want %v", inMap, tc.wantInMap)
			}
			if !tc.wantInMap {
				return
			}
			m := getAgentEntry(t, result, "rl-agent")
			if m["waiting_for_rate_limit"] != true {
				t.Errorf("waiting_for_rate_limit = %v, want true", m["waiting_for_rate_limit"])
			}
			if m["rate_limit_until_ts"] != tc.ts {
				t.Errorf("rate_limit_until_ts = %v, want %q", m["rate_limit_until_ts"], tc.ts)
			}
			if m["rate_limit_retry_count"].(int) != tc.retryCount {
				t.Errorf("rate_limit_retry_count = %v, want %d", m["rate_limit_retry_count"], tc.retryCount)
			}
		})
	}
}

// TestBuildActiveAgentsMap_RunningAndRateLimitCoexist verifies running and rate-limited
// sessions coexist in the map; running session has no waiting_for_rate_limit field.
func TestBuildActiveAgentsMap_RunningAndRateLimitCoexist(t *testing.T) {
	t.Parallel()
	fixedNow := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	futureTs := fixedNow.Add(time.Hour).Format(time.RFC3339Nano)

	pool, svc, wfiID := setupDeriveTestEnvWithClock(t, clock.NewTest(fixedNow))
	insertSession(t, pool, "s-run", wfiID, "runner-agent", "running", "", "")
	insertSessionRateLimit(t, pool, "s-rl", wfiID, "rl-agent", futureTs, 1, "")

	result := svc.buildActiveAgentsMap(wfiID, map[string][]RestartDetail{})
	if len(result) != 2 {
		t.Fatalf("map len = %d, want 2", len(result))
	}
	runEntry := getAgentEntry(t, result, "runner-agent")
	if _, exists := runEntry["waiting_for_rate_limit"]; exists {
		t.Errorf("running entry must not have waiting_for_rate_limit field")
	}
	rlEntry := getAgentEntry(t, result, "rl-agent")
	if rlEntry["waiting_for_rate_limit"] != true {
		t.Errorf("rl-agent must have waiting_for_rate_limit=true")
	}
}

// TestDeriveCurrentPhase_RateLimit verifies that a continued+future session
// returns its phase, while a continued+past session returns empty string.
func TestDeriveCurrentPhase_RateLimit(t *testing.T) {
	t.Parallel()
	fixedNow := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	futureTs := fixedNow.Add(time.Hour).Format(time.RFC3339Nano)
	pastTs := fixedNow.Add(-time.Hour).Format(time.RFC3339Nano)

	tests := []struct {
		name string
		ts   string
		want string
	}{
		{"future_returns_phase", futureTs, "analyzer"},
		{"past_returns_empty", pastTs, ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pool, svc, wfiID := setupDeriveTestEnvWithClock(t, clock.NewTest(fixedNow))
			insertSessionRateLimit(t, pool, "s1", wfiID, "analyzer", tc.ts, 1, "")

			got := svc.deriveCurrentPhase(wfiID)
			if got != tc.want {
				t.Errorf("deriveCurrentPhase() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestDeriveCurrentPhase_RateLimitVsRunning verifies that the session with the
// later created_at wins when both a running and a rate-limited session exist.
func TestDeriveCurrentPhase_RateLimitVsRunning(t *testing.T) {
	t.Parallel()
	fixedNow := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	futureTs := fixedNow.Add(time.Hour).Format(time.RFC3339Nano)

	pool, svc, wfiID := setupDeriveTestEnvWithClock(t, clock.NewTest(fixedNow))
	// running at t1, rate-limited at t2 > t1 → rate-limited phase wins (newer created_at)
	insertSession(t, pool, "s-run", wfiID, "analyzer", "running", "", "2025-06-01T10:00:00Z")
	insertSessionRateLimit(t, pool, "s-rl", wfiID, "builder", futureTs, 1, "2025-06-01T11:00:00Z")

	got := svc.deriveCurrentPhase(wfiID)
	if got != "builder" {
		t.Errorf("deriveCurrentPhase() = %q, want builder (rate-limited row is newer)", got)
	}
}
