package api

import (
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
)

// newSystemAgentRunsServer creates a minimal Server backed by the shared
// api-package template DB, mirroring newTierModelsServer.
func newSystemAgentRunsServer(t *testing.T) *Server {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "system_agent_runs_handler_test.db")
	if err := apiCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return &Server{pool: pool, clock: clock.Real()}
}

func seedRunsProjectAndWFI(t *testing.T, s *Server) string {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := s.pool.Exec(`INSERT INTO projects (id, name, created_at, updated_at) VALUES ('proj', 'proj', ?, ?)`, now, now); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	if _, err := s.pool.Exec(`INSERT INTO workflows (project_id, id, description, scope_type, created_at, updated_at) VALUES ('proj', 'wf', '', 'ticket', ?, ?)`, now, now); err != nil {
		t.Fatalf("seed workflow: %v", err)
	}
	wfiID := "wfi-runs"
	if _, err := s.pool.Exec(`INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, scope_type, created_at, updated_at) VALUES (?, 'proj', 'TKT-1', 'wf', 'active', 'ticket', ?, ?)`, wfiID, now, now); err != nil {
		t.Fatalf("seed workflow instance: %v", err)
	}
	return wfiID
}

func seedRunsAgentSession(t *testing.T, s *Server, id, wfiID, createdAt string, tier int) {
	t.Helper()
	// result='pass' avoids the ListSystemAgentRuns NULL-result scan bug (see
	// be_production_bugs) so these merge/order/filter tests exercise only
	// what they target.
	if _, err := s.pool.Exec(
		`INSERT INTO agent_sessions
			(id, project_id, ticket_id, workflow_instance_id, phase, agent_type, model_id, status, result,
			 tier, resolved_provider, resolved_execution_mode, resolved_effort, chain_position, fallback_from,
			 tokens_json, cost_estimate, created_at, updated_at)
		 VALUES (?, 'proj', 'TKT-1', ?, 'p', 'impl', 'sonnet-5', 'completed', 'pass', ?, 'anthropic', 'api', 'low', 1, ?, ?, ?, ?, ?)`,
		id, wfiID, tier, `[{"provider":"badprov"}]`, `{"input_tokens":5}`, 1.5, createdAt, createdAt,
	); err != nil {
		t.Fatalf("seed agent session %s: %v", id, err)
	}
}

func seedOrdinaryPhaseSession(t *testing.T, s *Server, id, wfiID, createdAt string) {
	t.Helper()
	if _, err := s.pool.Exec(
		`INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type, model_id, status, created_at, updated_at)
		 VALUES (?, 'proj', 'TKT-1', ?, 'p', 'impl', 'sonnet-5', 'completed', ?, ?)`,
		id, wfiID, createdAt, createdAt,
	); err != nil {
		t.Fatalf("seed ordinary session %s: %v", id, err)
	}
}

func seedRefineryRun(t *testing.T, s *Server, sessionID, foldedAt string) {
	t.Helper()
	if _, err := s.pool.Exec(
		`INSERT INTO refinery_runs (session_id, project_id, provider, model, prompt_tokens, output_tokens, status, folded_at)
		 VALUES (?, 'proj', 'anthropic', 'haiku-4-5', 10, 2, 'ok', ?)`,
		sessionID, foldedAt,
	); err != nil {
		t.Fatalf("seed refinery_run %s: %v", sessionID, err)
	}
}

func decodeRunsResponse(t *testing.T, rr *httptest.ResponseRecorder) (items []map[string]interface{}, limit int) {
	t.Helper()
	var body struct {
		Items []map[string]interface{} `json:"items"`
		Limit int                      `json:"limit"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return body.Items, body.Limit
}

func containsLiteralEmptyItems(body string) bool {
	return jsonContains(body, `"items":[]`)
}

func jsonContains(body, substr string) bool {
	for i := 0; i+len(substr) <= len(body); i++ {
		if body[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
