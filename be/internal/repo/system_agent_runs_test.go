package repo

import (
	"testing"
	"time"

	"be/internal/db"
	"be/internal/model"
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
	for _, run := range got {
		if run.TicketID != "TKT-1" {
			t.Errorf("run %s TicketID = %q, want TKT-1 (from agent_sessions.ticket_id)", run.SessionID, run.TicketID)
		}
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

// TestListSystemAgentRuns_NullResultSurfacesEmptyString verifies a
// still-running tiered session (NULL result) is scanned successfully with
// Result=="" rather than erroring the whole listing (formerly
// TestListSystemAgentRuns_NullResult_ProductionBug, which asserted the bug).
func TestListSystemAgentRuns_NullResultSurfacesEmptyString(t *testing.T) {
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

	got, err := r.ListSystemAgentRuns(50, time.Time{})
	if err != nil {
		t.Fatalf("ListSystemAgentRuns: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	if got[0].SessionID != "sess-null-result" {
		t.Errorf("got[0].SessionID = %q, want sess-null-result", got[0].SessionID)
	}
	if got[0].Result != "" {
		t.Errorf("got[0].Result = %q, want empty string for NULL result column", got[0].Result)
	}
	if got[0].TicketID != "TKT-1" {
		t.Errorf("got[0].TicketID = %q, want TKT-1", got[0].TicketID)
	}
}

// TestListSystemAgentRuns_ConsultChild verifies a `_consult` child session
// (nil tier, not a system_agent_definitions row) is surfaced by the widened
// WHERE (node_id = '_consult'), with its tokens/cost coming from the same
// agent_sessions row.
func TestListSystemAgentRuns_ConsultChild(t *testing.T) {
	t.Parallel()
	database, r, wfiID := setupTokenTestDB(t)
	defer database.Close()

	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := database.Exec(
		`INSERT INTO agent_sessions
			(id, project_id, ticket_id, workflow_instance_id, phase, node_id, agent_type, model_id, status, result,
			 tokens_json, cost_estimate, created_at, updated_at)
		 VALUES ('sess-consult', 'proj', 'TKT-1', ?, '_consult', '_consult', 'security-consultant', 'sonnet-5', 'completed', 'pass',
			 ?, ?, ?, ?)`,
		wfiID, `{"input_tokens":42}`, 0.25, now, now,
	); err != nil {
		t.Fatalf("insert consult child session: %v", err)
	}
	// Ordinary non-tiered session must still be excluded.
	insertAgentSessionForRuns(t, database, wfiID, "sess-ordinary", "impl", nil, now)

	got, err := r.ListSystemAgentRuns(50, time.Time{})
	if err != nil {
		t.Fatalf("ListSystemAgentRuns: %v", err)
	}

	var consult *model.SystemAgentRun
	for _, run := range got {
		if run.SessionID == "sess-consult" {
			consult = run
		}
		if run.SessionID == "sess-ordinary" {
			t.Errorf("sess-ordinary should be excluded (NULL tier, non-system agent_type, not _consult)")
		}
	}
	if consult == nil {
		t.Fatalf("sess-consult not found in %+v", got)
	}
	if consult.Tier != nil {
		t.Errorf("Tier = %v, want nil for a consult child", *consult.Tier)
	}
	if consult.CostEstimate == nil || *consult.CostEstimate != 0.25 {
		t.Errorf("CostEstimate = %v, want 0.25", consult.CostEstimate)
	}
	if string(consult.TokensJSON) != `{"input_tokens":42}` {
		t.Errorf("TokensJSON = %s, want {\"input_tokens\":42}", consult.TokensJSON)
	}
	if consult.AgentType != "security-consultant" {
		t.Errorf("AgentType = %q, want security-consultant (the consultant's agent_definitions id)", consult.AgentType)
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
