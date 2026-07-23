package service

import (
	"testing"

	"be/internal/model"
)

// TestResolveDefChain_MatchesSystemAgent proves ResolveDefChain (agent_def_chain.go)
// and the SystemAgentDefinition adapter (system_agent_chain.go) resolve the
// identical chain for the same tier — both are thin wrappers over the single
// shared resolveChain implementation (Rule 6: polymorphism lives in the
// implementation).
func TestResolveDefChain_MatchesSystemAgent(t *testing.T) {
	t.Parallel()
	sysSvc, cleanup := setupSysAgentDefTestEnv(t)
	t.Cleanup(cleanup)

	tier := 1
	sysDef := &model.SystemAgentDefinition{ID: "sys-def", ExecutionMode: "api", Tier: &tier}
	sysChain, err := sysSvc.ResolveAgentChain(sysDef)
	if err != nil {
		t.Fatalf("SystemAgentDefinitionService.ResolveAgentChain: %v", err)
	}

	wfDef := &model.AgentDefinition{ID: "wf-def", ExecutionMode: "api", Tier: &tier}
	wfChain, err := ResolveDefChain(sysSvc.pool, sysSvc.clock, sysSvc.modelSvc, wfDef)
	if err != nil {
		t.Fatalf("ResolveDefChain: %v", err)
	}

	if len(sysChain) != len(wfChain) {
		t.Fatalf("chain length mismatch: system=%d workflow=%d", len(sysChain), len(wfChain))
	}
	for i := range sysChain {
		if sysChain[i] != wfChain[i] {
			t.Errorf("entry %d mismatch: system=%+v workflow=%+v", i, sysChain[i], wfChain[i])
		}
	}
}

// TestResolveDefChain_Override verifies ResolveDefChain's override path
// (def.Model != "").
func TestResolveDefChain_Override(t *testing.T) {
	t.Parallel()
	sysSvc, cleanup := setupSysAgentDefTestEnv(t)
	t.Cleanup(cleanup)
	createTestModel(t, sysSvc, "wf-override-model", "low")

	def := &model.AgentDefinition{ID: "wf-def", ExecutionMode: "api", Model: "wf-override-model"}
	chain, err := ResolveDefChain(sysSvc.pool, sysSvc.clock, sysSvc.modelSvc, def)
	if err != nil {
		t.Fatalf("ResolveDefChain: %v", err)
	}
	if len(chain) != 1 || chain[0].ModelID != "wf-override-model" {
		t.Errorf("chain = %+v, want single entry wf-override-model", chain)
	}
}
