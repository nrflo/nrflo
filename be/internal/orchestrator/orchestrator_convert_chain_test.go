package orchestrator

import (
	"testing"

	"be/internal/service"
)

// TestConvertToSpawnerAgents_PreservesChain is a regression guard for the
// easiest bug in the tier-chain plumbing: convertToSpawnerAgents must carry
// service.SpawnerAgentConfig.Chain through to spawner.AgentConfig.Chain
// verbatim, not just Model/Timeout/ReasoningEffort.
func TestConvertToSpawnerAgents_PreservesChain(t *testing.T) {
	t.Parallel()
	chain := []service.AgentChainEntry{
		{Provider: "anthropic", ExecutionMode: "api", ModelID: "sonnet-5", ReasoningEffort: "low", Tier: 2},
		{Provider: "anthropic", ExecutionMode: "cli_interactive", ModelID: "sonnet-5", ReasoningEffort: "low", Tier: 2},
	}
	effort := "low"
	svcAgents := map[string]service.SpawnerAgentConfig{
		"tiered": {Model: "sonnet-5", Timeout: 20, ReasoningEffort: &effort, Chain: chain},
		"plain":  {Model: "opus-4-8", Timeout: 30},
	}

	got := convertToSpawnerAgents(svcAgents)

	tiered, ok := got["tiered"]
	if !ok {
		t.Fatal("converted agents missing 'tiered' entry")
	}
	if len(tiered.Chain) != len(chain) {
		t.Fatalf("tiered.Chain length = %d, want %d", len(tiered.Chain), len(chain))
	}
	for i := range chain {
		if tiered.Chain[i] != chain[i] {
			t.Errorf("tiered.Chain[%d] = %+v, want %+v", i, tiered.Chain[i], chain[i])
		}
	}

	plain, ok := got["plain"]
	if !ok {
		t.Fatal("converted agents missing 'plain' entry")
	}
	if plain.Chain != nil {
		t.Errorf("plain.Chain = %+v, want nil (no chain resolved)", plain.Chain)
	}
}
