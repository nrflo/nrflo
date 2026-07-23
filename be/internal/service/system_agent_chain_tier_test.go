package service

import "testing"

// TestResolveAgentChain_TierChainCarriesResolvedTier verifies that
// loadTierChain's entries carry the tier actually loaded, including the
// inheritance walk-down case (def.Tier=3 with only tier-1 rows seeded).
func TestResolveAgentChain_TierChainCarriesResolvedTier(t *testing.T) {
	svc, cleanup := setupSysAgentDefTestEnv(t)
	t.Cleanup(cleanup)

	t.Run("direct tier match carries that tier", func(t *testing.T) {
		def := tierDef(intPtr(1))
		chain, err := svc.ResolveAgentChain(def)
		if err != nil {
			t.Fatalf("ResolveAgentChain: %v", err)
		}
		for i, e := range chain {
			if e.Tier != 1 {
				t.Errorf("chain[%d].Tier = %d, want 1 (direct match)", i, e.Tier)
			}
		}
	})

	t.Run("inheritance walk-down carries the tier actually loaded, not the requested one", func(t *testing.T) {
		// Tier 2 has no seeded rows; resolution walks down to tier 1's chain.
		def := tierDef(intPtr(2))
		chain, err := svc.ResolveAgentChain(def)
		if err != nil {
			t.Fatalf("ResolveAgentChain: %v", err)
		}
		if len(chain) == 0 {
			t.Fatal("chain is empty, want the inherited tier1 chain")
		}
		for i, e := range chain {
			if e.Tier != 1 {
				t.Errorf("chain[%d].Tier = %d, want 1 (inherited tier, not requested tier 2)", i, e.Tier)
			}
		}
	})
}

// TestResolveAgentChain_OverrideEntryTier verifies an override entry carries
// def.Tier verbatim, defaulting to 0 when def.Tier is nil.
func TestResolveAgentChain_OverrideEntryTier(t *testing.T) {
	svc, cleanup := setupSysAgentDefTestEnv(t)
	t.Cleanup(cleanup)
	createTestModel(t, svc, "override-tier-model", "low")

	t.Run("nil def.Tier yields Tier=0", func(t *testing.T) {
		def := overrideDef("override-tier-model", "api", nil)
		chain, err := svc.ResolveAgentChain(def)
		if err != nil {
			t.Fatalf("ResolveAgentChain: %v", err)
		}
		if len(chain) != 1 {
			t.Fatalf("chain length = %d, want 1", len(chain))
		}
		if chain[0].Tier != 0 {
			t.Errorf("chain[0].Tier = %d, want 0 (def.Tier nil)", chain[0].Tier)
		}
	})

	t.Run("non-nil def.Tier is carried through even for an override entry", func(t *testing.T) {
		def := overrideDef("override-tier-model", "api", nil)
		def.Tier = intPtr(3)
		chain, err := svc.ResolveAgentChain(def)
		if err != nil {
			t.Fatalf("ResolveAgentChain: %v", err)
		}
		if len(chain) != 1 {
			t.Fatalf("chain length = %d, want 1", len(chain))
		}
		if chain[0].Tier != 3 {
			t.Errorf("chain[0].Tier = %d, want 3 (def.Tier=3)", chain[0].Tier)
		}
	})
}
