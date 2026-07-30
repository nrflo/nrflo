package db

import "testing"

// 000222 appends a gpt-5.6-terra codex hop to the sonnet tiers (2-4) and
// clears the model pin on every system agent whose pin equaled its tier
// chain's head, so the chains actually fire. context-saver keeps its
// deliberate sonnet-5 pin (its tier-1 head is haiku).

func TestMigration222_SonnetTiersGainTerraHop(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	for tier, effort := range map[int]string{2: "low", 3: "medium", 4: "medium"} {
		var provider, mode, model, gotEffort string
		err := pool.QueryRow(
			`SELECT provider, execution_mode, model_id, reasoning_effort FROM tier_models WHERE tier = ? AND position = 2`, tier,
		).Scan(&provider, &mode, &model, &gotEffort)
		if err != nil {
			t.Fatalf("tier %d position 2: %v", tier, err)
		}
		if provider != "openai" || mode != "cli_interactive" || model != "gpt-5.6-terra" || gotEffort != effort {
			t.Errorf("tier %d hop = %s/%s/%s/%s, want openai/cli_interactive/gpt-5.6-terra/%s",
				tier, provider, mode, model, gotEffort, effort)
		}
	}
}

func TestMigration222_HeadEqualPinsCleared(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	rows, err := pool.Query(`SELECT id, model FROM system_agent_definitions`)
	if err != nil {
		t.Fatalf("SELECT system_agent_definitions: %v", err)
	}
	defer rows.Close()
	got := map[string]string{}
	for rows.Next() {
		var id, model string
		if err := rows.Scan(&id, &model); err != nil {
			t.Fatalf("scan: %v", err)
		}
		got[id] = model
	}
	for _, id := range []string{"_refinery", "_t2_extractor", "_t1_executor", "planner-system", "planner-system-api", "conflict-resolver", "spec-normalizer", "context-saver-api", "context-saver"} {
		if got[id] != "" {
			t.Errorf("%s model = %q, want empty (tier-resolved)", id, got[id])
		}
	}
}

func TestMigration222_Tier5Seeded(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	rows, err := pool.Query(`SELECT position, provider, model_id, reasoning_effort FROM tier_models WHERE tier = 5 ORDER BY position`)
	if err != nil {
		t.Fatalf("SELECT tier 5: %v", err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var pos int
		var provider, model, effort string
		if err := rows.Scan(&pos, &provider, &model, &effort); err != nil {
			t.Fatalf("scan: %v", err)
		}
		got = append(got, provider+"/"+model+"/"+effort)
	}
	want := []string{"anthropic/opus-5/high", "anthropic/opus-5/high", "openai/gpt-5.6-sol/high"}
	if len(got) != len(want) {
		t.Fatalf("tier 5 = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("tier 5 position %d = %s, want %s", i, got[i], want[i])
		}
	}
}
