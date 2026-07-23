package repo

import (
	"database/sql"
	"testing"
)

// TestUpdateTierResolution_RoundTrip verifies tier/provider/mode/effort/
// chain_position/fallback_from all read back after UpdateTierResolution.
func TestUpdateTierResolution_RoundTrip(t *testing.T) {
	t.Parallel()
	database, r, wfiID := setupTokenTestDB(t)
	defer database.Close()

	insertSessionWithToken(t, database, "sess-tier-1", wfiID, "tok-tier-1", "running")

	tier := 2
	fallbackFrom := `[{"provider":"badprov","model_id":"bad-model"}]`
	if err := r.UpdateTierResolution("sess-tier-1", &tier, "goodprov", "api", "low", 1, fallbackFrom); err != nil {
		t.Fatalf("UpdateTierResolution: %v", err)
	}

	var gotTier sql.NullInt64
	var gotProvider, gotMode, gotEffort, gotFallback string
	var gotPos int
	if err := database.QueryRow(
		`SELECT tier, resolved_provider, resolved_execution_mode, resolved_effort, chain_position, fallback_from
		 FROM agent_sessions WHERE id = ?`, "sess-tier-1",
	).Scan(&gotTier, &gotProvider, &gotMode, &gotEffort, &gotPos, &gotFallback); err != nil {
		t.Fatalf("query: %v", err)
	}
	if !gotTier.Valid || gotTier.Int64 != 2 {
		t.Errorf("tier = %+v, want 2", gotTier)
	}
	if gotProvider != "goodprov" {
		t.Errorf("resolved_provider = %q, want goodprov", gotProvider)
	}
	if gotMode != "api" {
		t.Errorf("resolved_execution_mode = %q, want api", gotMode)
	}
	if gotEffort != "low" {
		t.Errorf("resolved_effort = %q, want low", gotEffort)
	}
	if gotPos != 1 {
		t.Errorf("chain_position = %d, want 1", gotPos)
	}
	if gotFallback != fallbackFrom {
		t.Errorf("fallback_from = %q, want %q", gotFallback, fallbackFrom)
	}
}

// TestUpdateTierResolution_NilTierStaysNull verifies a nil tier pointer
// writes NULL rather than 0.
func TestUpdateTierResolution_NilTierStaysNull(t *testing.T) {
	t.Parallel()
	database, r, wfiID := setupTokenTestDB(t)
	defer database.Close()

	insertSessionWithToken(t, database, "sess-tier-nil", wfiID, "tok-tier-nil", "running")

	if err := r.UpdateTierResolution("sess-tier-nil", nil, "anthropic", "cli_interactive", "medium", 0, ""); err != nil {
		t.Fatalf("UpdateTierResolution: %v", err)
	}

	var gotTier sql.NullInt64
	if err := database.QueryRow(`SELECT tier FROM agent_sessions WHERE id = ?`, "sess-tier-nil").Scan(&gotTier); err != nil {
		t.Fatalf("query: %v", err)
	}
	if gotTier.Valid {
		t.Errorf("tier = %+v, want NULL for nil tier pointer", gotTier)
	}
}

// TestUpdateTierResolution_EmptyFallbackFromStaysNull verifies an empty
// fallbackFrom string (chain position 0, no entries tried before it) writes
// NULL rather than an empty string.
func TestUpdateTierResolution_EmptyFallbackFromStaysNull(t *testing.T) {
	t.Parallel()
	database, r, wfiID := setupTokenTestDB(t)
	defer database.Close()

	insertSessionWithToken(t, database, "sess-tier-fb", wfiID, "tok-tier-fb", "running")

	if err := r.UpdateTierResolution("sess-tier-fb", nil, "anthropic", "api", "low", 0, ""); err != nil {
		t.Fatalf("UpdateTierResolution: %v", err)
	}

	var gotFallback sql.NullString
	if err := database.QueryRow(`SELECT fallback_from FROM agent_sessions WHERE id = ?`, "sess-tier-fb").Scan(&gotFallback); err != nil {
		t.Fatalf("query: %v", err)
	}
	if gotFallback.Valid {
		t.Errorf("fallback_from = %+v, want NULL for empty fallbackFrom", gotFallback)
	}
}

// TestUpdateTierResolution_UnknownID_ReturnsNilError verifies a targeted
// UPDATE against a missing session id is a silent no-op, mirroring UpdateCost.
func TestUpdateTierResolution_UnknownID_ReturnsNilError(t *testing.T) {
	t.Parallel()
	_, r, _ := setupTokenTestDB(t)

	tier := 1
	if err := r.UpdateTierResolution("does-not-exist", &tier, "anthropic", "api", "low", 0, ""); err != nil {
		t.Errorf("UpdateTierResolution on unknown session = %v, want nil error", err)
	}
}
