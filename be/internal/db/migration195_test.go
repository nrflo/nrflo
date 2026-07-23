package db

import "testing"

// TestMigration195_TierModelsSchema verifies the tier_models table exists
// with the expected columns and (tier, position) primary key.
func TestMigration195_TierModelsSchema(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	rows, err := pool.Query(`PRAGMA table_info(tier_models)`)
	if err != nil {
		t.Fatalf("PRAGMA table_info: %v", err)
	}
	defer rows.Close()

	wantCols := map[string]bool{
		"tier": false, "position": false, "provider": false,
		"execution_mode": false, "model_id": false, "reasoning_effort": false,
	}
	pkCols := map[string]int{}
	for rows.Next() {
		var cid, notnull, pk int
		var name, ctype string
		var dflt any
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			t.Fatalf("scan table_info row: %v", err)
		}
		if _, ok := wantCols[name]; ok {
			wantCols[name] = true
		}
		if pk > 0 {
			pkCols[name] = pk
		}
	}
	for col, found := range wantCols {
		if !found {
			t.Errorf("tier_models missing column %q", col)
		}
	}
	if len(pkCols) != 2 || pkCols["tier"] == 0 || pkCols["position"] == 0 {
		t.Errorf("tier_models primary key = %v, want composite (tier, position)", pkCols)
	}
}

// TestMigration195_TierColumnOnSystemAgentDefinitions verifies the nullable
// tier column was added to system_agent_definitions.
func TestMigration195_TierColumnOnSystemAgentDefinitions(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	rows, err := pool.Query(`PRAGMA table_info(system_agent_definitions)`)
	if err != nil {
		t.Fatalf("PRAGMA table_info: %v", err)
	}
	defer rows.Close()

	found := false
	for rows.Next() {
		var cid, notnull, pk int
		var name, ctype string
		var dflt any
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			t.Fatalf("scan table_info row: %v", err)
		}
		if name == "tier" {
			found = true
			if notnull != 0 {
				t.Errorf("tier notnull = %d, want 0 (nullable)", notnull)
			}
		}
	}
	if !found {
		t.Fatal("system_agent_definitions missing tier column")
	}
}

// TestMigration195_TierBackfill verifies the 9 system agents were backfilled
// to the intended tier: haiku-class agents to tier 1, sonnet-class to tier 4.
func TestMigration195_TierBackfill(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	wantTier := map[string]int{
		"_refinery":          1,
		"_t2_extractor":      1,
		"context-saver":      1,
		"context-saver-api":  1,
		"spec-normalizer":    1,
		"_t1_executor":       4,
		"conflict-resolver":  4,
		"planner-system":     4,
		"planner-system-api": 4,
	}
	for id, want := range wantTier {
		var tier int
		if err := pool.QueryRow(`SELECT tier FROM system_agent_definitions WHERE id = ?`, id).Scan(&tier); err != nil {
			t.Fatalf("query tier for %s: %v", id, err)
		}
		if tier != want {
			t.Errorf("agent %s tier = %d, want %d", id, tier, want)
		}
	}
}

// TestMigration195_TierChainSeed verifies the seeded tier1/tier4 fallback
// chains: position 0 = anthropic/api, position 1 = anthropic/cli_interactive,
// with the expected model + reasoning effort at each position. Position 0's
// execution_mode is rewritten to ” (inherit) by migration 000200 — read on
// the fully-migrated pool, this test observes that post-000200 state.
func TestMigration195_TierChainSeed(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	type want struct {
		provider, mode, model, effort string
	}
	cases := []struct {
		tier, position int
		want           want
	}{
		{1, 0, want{"anthropic", "", "haiku-4-5", "low"}},
		{1, 1, want{"anthropic", "cli_interactive", "haiku-4-5", "low"}},
		{4, 0, want{"anthropic", "", "sonnet-5", "medium"}},
		{4, 1, want{"anthropic", "cli_interactive", "sonnet-5", "medium"}},
	}

	for _, c := range cases {
		var provider, mode, modelID, effort string
		err := pool.QueryRow(
			`SELECT provider, execution_mode, model_id, reasoning_effort
			 FROM tier_models WHERE tier = ? AND position = ?`, c.tier, c.position,
		).Scan(&provider, &mode, &modelID, &effort)
		if err != nil {
			t.Fatalf("query tier=%d position=%d: %v", c.tier, c.position, err)
		}
		got := want{provider, mode, modelID, effort}
		if got != c.want {
			t.Errorf("tier=%d position=%d = %+v, want %+v", c.tier, c.position, got, c.want)
		}
	}

	// Exactly two positions per seeded tier — no stray fallback rows.
	for _, tier := range []int{1, 4} {
		var count int
		if err := pool.QueryRow(`SELECT COUNT(*) FROM tier_models WHERE tier = ?`, tier).Scan(&count); err != nil {
			t.Fatalf("count tier=%d: %v", tier, err)
		}
		if count != 2 {
			t.Errorf("tier=%d row count = %d, want 2", tier, count)
		}
	}
}
