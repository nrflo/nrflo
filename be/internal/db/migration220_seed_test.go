package db

import (
	"strings"
	"testing"
)

// 000220 re-seeds the _t2_extractor for tier-chain resolution: no model pin
// (empty model resolves the tier-1 tier_models ladder), native fs tools in
// the CSV, a doubled iteration budget, a search-budget prompt rule, and a
// cross-provider codex hop appended to the default tier-1 chain.

func TestMigration220_ExtractorTierResolved(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	var model, tools, prompt string
	var tier, maxIter int
	err = pool.QueryRow(
		`SELECT model, tier, api_max_iterations, tools, prompt FROM system_agent_definitions WHERE id = '_t2_extractor'`,
	).Scan(&model, &tier, &maxIter, &tools, &prompt)
	if err != nil {
		t.Fatalf("SELECT _t2_extractor: %v", err)
	}
	if model != "" {
		t.Errorf("model = %q, want empty (tier-resolved)", model)
	}
	if tier != 1 {
		t.Errorf("tier = %d, want 1", tier)
	}
	if maxIter != 12 {
		t.Errorf("api_max_iterations = %d, want 12", maxIter)
	}
	for _, tool := range []string{"read_file", "bash", "findings_add", "agent_finished"} {
		if !strings.Contains(tools, tool) {
			t.Errorf("tools CSV missing %q: %s", tool, tools)
		}
	}
	for _, anchor := range []string{"Budget your search", "_delegate_findings", "${DELEGATE_BRIEF}"} {
		if !strings.Contains(prompt, anchor) {
			t.Errorf("prompt missing anchor %q", anchor)
		}
	}
}

func TestMigration220_Tier1ChainGainsCodexHop(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	rows, err := pool.Query(
		`SELECT position, provider, execution_mode, model_id, reasoning_effort FROM tier_models WHERE tier = 1 ORDER BY position`)
	if err != nil {
		t.Fatalf("SELECT tier_models: %v", err)
	}
	defer rows.Close()
	type entry struct {
		pos                           int
		provider, mode, model, effort string
	}
	var chain []entry
	for rows.Next() {
		var e entry
		if err := rows.Scan(&e.pos, &e.provider, &e.mode, &e.model, &e.effort); err != nil {
			t.Fatalf("scan: %v", err)
		}
		chain = append(chain, e)
	}
	if len(chain) != 3 {
		t.Fatalf("tier-1 chain length = %d, want 3 (haiku api, haiku cli, luna codex): %+v", len(chain), chain)
	}
	if chain[2].provider != "openai" || chain[2].mode != "cli_interactive" || chain[2].model != "gpt-5.6-luna" || chain[2].effort != "low" {
		t.Errorf("position 2 = %+v, want openai/cli_interactive/gpt-5.6-luna/low", chain[2])
	}
}
