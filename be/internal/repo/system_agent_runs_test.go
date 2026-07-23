package repo

import (
	"testing"
	"time"

	"be/internal/db"
)

// insertAgentSessionForRuns inserts an agent_sessions row shaped for
// ListSystemAgentRuns tests: explicit created_at (ordering/since control),
// optional tier, and the columns the merged listing surfaces.
func insertAgentSessionForRuns(t *testing.T, database *db.DB, wfiID, id, agentType string, tier *int, createdAt string) {
	t.Helper()
	// result is set (not NULL) here: a NULL result is a separate concern
	// (see be_production_bugs — ListSystemAgentRuns.Scan currently rejects a
	// NULL result column) that these ordering/filter tests don't target.
	_, err := database.Exec(
		`INSERT INTO agent_sessions
			(id, project_id, ticket_id, workflow_instance_id, phase, agent_type, model_id, status, result,
			 tier, resolved_provider, resolved_execution_mode, resolved_effort, chain_position,
			 tokens_json, cost_estimate, created_at, updated_at)
		 VALUES (?, 'proj', 'TKT-1', ?, 'p', ?, 'sonnet-5', 'completed', 'pass', ?, 'anthropic', 'api', 'low', 0, ?, ?, ?, ?)`,
		id, wfiID, agentType, tier, `{"input_tokens":10}`, 0.5, createdAt, createdAt,
	)
	if err != nil {
		t.Fatalf("insert runs session %s: %v", id, err)
	}
}

func TestListSystemAgentRuns(t *testing.T) {
	t.Parallel()
	database, r, wfiID := setupTokenTestDB(t)
	defer database.Close()

	if _, err := database.Exec(
		`INSERT INTO system_agent_definitions (id, model, timeout, prompt, role, execution_mode, created_at, updated_at)
		 VALUES ('_refinery_test', 'haiku-4-5', 3, 'p', 'test_runs_role', 'api', datetime('now'), datetime('now'))`,
	); err != nil {
		t.Fatalf("seed system_agent_definitions: %v", err)
	}

	t1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tier1, tier2 := 1, 2
	insertAgentSessionForRuns(t, database, wfiID, "sess-tier", "impl", &tier2, t1.Format(time.RFC3339Nano))
	insertAgentSessionForRuns(t, database, wfiID, "sess-systype", "_refinery_test", nil, t1.Add(time.Minute).Format(time.RFC3339Nano))
	insertAgentSessionForRuns(t, database, wfiID, "sess-ordinary", "impl", nil, t1.Add(2*time.Minute).Format(time.RFC3339Nano))
	insertAgentSessionForRuns(t, database, wfiID, "sess-newest", "impl", &tier1, t1.Add(3*time.Minute).Format(time.RFC3339Nano))

	got, err := r.ListSystemAgentRuns(50, time.Time{})
	if err != nil {
		t.Fatalf("ListSystemAgentRuns: %v", err)
	}

	ids := make([]string, len(got))
	for i, run := range got {
		ids[i] = run.SessionID
	}
	if containsID(ids, "sess-ordinary") {
		t.Errorf("ids = %v, want sess-ordinary excluded (NULL tier, non-system agent_type)", ids)
	}
	if !containsID(ids, "sess-tier") {
		t.Errorf("ids = %v, want sess-tier included (tier set)", ids)
	}
	if !containsID(ids, "sess-systype") {
		t.Errorf("ids = %v, want sess-systype included (agent_type matches system_agent_definitions id)", ids)
	}
	if len(ids) < 3 || ids[0] != "sess-newest" {
		t.Errorf("ids = %v, want sess-newest first (newest created_at)", ids)
	}
}

func TestListSystemAgentRuns_Limit(t *testing.T) {
	t.Parallel()
	database, r, wfiID := setupTokenTestDB(t)
	defer database.Close()

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tier1 := 1
	names := []string{"sess-lim-a", "sess-lim-b", "sess-lim-c", "sess-lim-d", "sess-lim-e"}
	for i, name := range names {
		insertAgentSessionForRuns(t, database, wfiID, name, "impl", &tier1,
			base.Add(time.Duration(i)*time.Minute).Format(time.RFC3339Nano))
	}

	got, err := r.ListSystemAgentRuns(2, time.Time{})
	if err != nil {
		t.Fatalf("ListSystemAgentRuns: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2 (limit)", len(got))
	}
	if got[0].SessionID != "sess-lim-e" {
		t.Errorf("got[0].SessionID = %q, want sess-lim-e (newest)", got[0].SessionID)
	}
}

func TestListSystemAgentRuns_Since(t *testing.T) {
	t.Parallel()
	database, r, wfiID := setupTokenTestDB(t)
	defer database.Close()

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tier1 := 1
	insertAgentSessionForRuns(t, database, wfiID, "sess-old", "impl", &tier1, base.Format(time.RFC3339Nano))
	insertAgentSessionForRuns(t, database, wfiID, "sess-new", "impl", &tier1, base.Add(time.Hour).Format(time.RFC3339Nano))

	since := base.Add(30 * time.Minute)
	got, err := r.ListSystemAgentRuns(50, since)
	if err != nil {
		t.Fatalf("ListSystemAgentRuns: %v", err)
	}
	if len(got) != 1 || got[0].SessionID != "sess-new" {
		t.Fatalf("got = %+v, want only sess-new (since filter)", got)
	}
}

// TestListSystemAgentRuns_NullResult_ProductionBug documents a known bug:
// ListSystemAgentRuns.Scan reads agent_sessions.result (nullable) directly
// into a plain string field, so any qualifying row with a NULL result
// (e.g. a still-running tiered session) errors the whole listing instead of
// surfacing an empty Result. See be_production_bugs.
func TestListSystemAgentRuns_NullResult_ProductionBug(t *testing.T) {
	t.Parallel()
	database, r, wfiID := setupTokenTestDB(t)
	defer database.Close()

	now := time.Now().UTC().Format(time.RFC3339Nano)
	tier := 1
	if _, err := database.Exec(
		`INSERT INTO agent_sessions
			(id, project_id, ticket_id, workflow_instance_id, phase, agent_type, model_id, status,
			 tier, resolved_provider, resolved_execution_mode, resolved_effort, chain_position, created_at, updated_at)
		 VALUES ('sess-null-result', 'proj', 'TKT-1', ?, 'p', 'impl', 'sonnet-5', 'running', ?, 'anthropic', 'api', 'low', 0, ?, ?)`,
		wfiID, tier, now, now,
	); err != nil {
		t.Fatalf("insert running session with NULL result: %v", err)
	}

	if _, err := r.ListSystemAgentRuns(50, time.Time{}); err == nil {
		t.Error("ListSystemAgentRuns succeeded on a NULL-result row; if this now passes, the production bug is fixed — replace this test with a real assertion")
	}
}

func containsID(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
