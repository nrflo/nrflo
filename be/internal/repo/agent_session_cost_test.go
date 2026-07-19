package repo

import (
	"database/sql"
	"testing"
)

// TestUpdateCost_PersistsTokensJSONAndCostEstimate verifies UpdateCost writes
// both nullable columns via a targeted UPDATE (mirrors UpdateContextLeft).
func TestUpdateCost_PersistsTokensJSONAndCostEstimate(t *testing.T) {
	t.Parallel()
	database, r, wfiID := setupTokenTestDB(t)
	defer database.Close()

	insertSessionWithToken(t, database, "sess-cost", wfiID, "tok-cost", "running")

	tokensJSON := `{"input_tokens":1000,"output_tokens":200,"cache_read_tokens":0,"cache_write_tokens":0}`
	if err := r.UpdateCost("sess-cost", tokensJSON, 1.2345); err != nil {
		t.Fatalf("UpdateCost: %v", err)
	}

	var gotTokens sql.NullString
	var gotCost sql.NullFloat64
	if err := database.QueryRow(`SELECT tokens_json, cost_estimate FROM agent_sessions WHERE id = ?`, "sess-cost").
		Scan(&gotTokens, &gotCost); err != nil {
		t.Fatalf("query: %v", err)
	}
	if !gotTokens.Valid || gotTokens.String != tokensJSON {
		t.Errorf("tokens_json = %+v, want %q", gotTokens, tokensJSON)
	}
	if !gotCost.Valid || gotCost.Float64 != 1.2345 {
		t.Errorf("cost_estimate = %+v, want 1.2345", gotCost)
	}
}

// TestUpdateCost_UnknownSession_ReturnsNilError verifies UpdateCost is a
// silent no-op on 0 rows affected (a session that ended before its first
// flush) rather than erroring, unlike UpdateContextLeft's sibling methods
// that treat 0 rows as "not found".
func TestUpdateCost_UnknownSession_ReturnsNilError(t *testing.T) {
	t.Parallel()
	database, r, _ := setupTokenTestDB(t)
	defer database.Close()

	if err := r.UpdateCost("does-not-exist", `{}`, 5.0); err != nil {
		t.Errorf("UpdateCost on unknown session = %v, want nil error", err)
	}
}

// TestUpdateCost_OverwritesPreviousSnapshot verifies a second call replaces
// (not merges) the previous tokens_json/cost_estimate values.
func TestUpdateCost_OverwritesPreviousSnapshot(t *testing.T) {
	t.Parallel()
	database, r, wfiID := setupTokenTestDB(t)
	defer database.Close()

	insertSessionWithToken(t, database, "sess-cost-2", wfiID, "tok-cost-2", "running")

	if err := r.UpdateCost("sess-cost-2", `{"input_tokens":100}`, 0.5); err != nil {
		t.Fatalf("first UpdateCost: %v", err)
	}
	if err := r.UpdateCost("sess-cost-2", `{"input_tokens":900}`, 4.5); err != nil {
		t.Fatalf("second UpdateCost: %v", err)
	}

	var gotTokens string
	var gotCost float64
	if err := database.QueryRow(`SELECT tokens_json, cost_estimate FROM agent_sessions WHERE id = ?`, "sess-cost-2").
		Scan(&gotTokens, &gotCost); err != nil {
		t.Fatalf("query: %v", err)
	}
	if gotTokens != `{"input_tokens":900}` || gotCost != 4.5 {
		t.Errorf("tokens_json/cost_estimate = %q/%v, want overwritten values", gotTokens, gotCost)
	}
}
