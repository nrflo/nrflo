package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// seedRefineryRunWithChain seeds a refinery_runs row carrying a non-zero
// chain_position/execution_mode/fallback_from, mirroring a cli_interactive
// fallback landing recorded by fold_chain.go's walkFoldChain.
func seedRefineryRunWithChain(t *testing.T, s *Server, sessionID, foldedAt string, chainPos int, execMode, fallbackFrom string) {
	t.Helper()
	if _, err := s.pool.Exec(
		`INSERT INTO refinery_runs (session_id, project_id, provider, model, prompt_tokens, output_tokens, status, folded_at, chain_position, execution_mode, fallback_from)
		 VALUES (?, 'proj', 'openai', 'gpt-5.6-luna', 4, 6, 'ok', ?, ?, ?, ?)`,
		sessionID, foldedAt, chainPos, execMode, fallbackFrom,
	); err != nil {
		t.Fatalf("seed chained refinery_run %s: %v", sessionID, err)
	}
}

// TestHandleListSystemAgentRuns_RefineryFoldChainFields verifies a
// refinery_fold item surfaces chain_position/resolved_execution_mode/
// fallback_from from its refinery_runs row (previously left unfilled — see
// implementor notes on handlers_system_agent_runs.go).
func TestHandleListSystemAgentRuns_RefineryFoldChainFields(t *testing.T) {
	s := newSystemAgentRunsServer(t)
	seedRunsProjectAndWFI(t, s)

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	fallback := `[{"provider":"anthropic","execution_mode":"api","model_id":"haiku-4-5"}]`
	seedRefineryRunWithChain(t, s, "sess-fold-chained", base.Format(time.RFC3339Nano), 2, "cli_interactive", fallback)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/system-agent-runs", nil)
	rr := httptest.NewRecorder()
	s.handleListSystemAgentRuns(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	items, _ := decodeRunsResponse(t, rr)
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	it := items[0]
	if it["kind"] != "refinery_fold" {
		t.Errorf("kind = %v, want refinery_fold", it["kind"])
	}
	if it["chain_position"] != float64(2) {
		t.Errorf("chain_position = %v, want 2", it["chain_position"])
	}
	if it["resolved_execution_mode"] != "cli_interactive" {
		t.Errorf("resolved_execution_mode = %v, want cli_interactive", it["resolved_execution_mode"])
	}
	fb, ok := it["fallback_from"].([]interface{})
	if !ok || len(fb) != 1 {
		t.Fatalf("fallback_from = %v, want a one-element array", it["fallback_from"])
	}
	entry, ok := fb[0].(map[string]interface{})
	if !ok || entry["provider"] != "anthropic" {
		t.Errorf("fallback_from[0] = %v, want an object with provider=anthropic", fb[0])
	}
}

// TestHandleListSystemAgentRuns_RefineryFoldChainFields_ZeroValuesOmitted
// verifies a position-0 (primary) fold row omits chain_position/
// fallback_from/resolved_execution_mode (all zero-valued) from the JSON
// response, matching the agent_session rows' omitempty behavior.
func TestHandleListSystemAgentRuns_RefineryFoldChainFields_ZeroValuesOmitted(t *testing.T) {
	s := newSystemAgentRunsServer(t)
	seedRunsProjectAndWFI(t, s)

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	seedRefineryRun(t, s, "sess-fold-primary", base.Format(time.RFC3339Nano))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/system-agent-runs", nil)
	rr := httptest.NewRecorder()
	s.handleListSystemAgentRuns(rr, req)

	items, _ := decodeRunsResponse(t, rr)
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	it := items[0]
	if _, present := it["chain_position"]; present {
		t.Errorf("chain_position = %v, want absent (omitempty at position 0)", it["chain_position"])
	}
	if _, present := it["fallback_from"]; present {
		t.Errorf("fallback_from = %v, want absent (omitempty when empty)", it["fallback_from"])
	}
	if _, present := it["resolved_execution_mode"]; present {
		t.Errorf("resolved_execution_mode = %v, want absent (omitempty when empty)", it["resolved_execution_mode"])
	}
}
