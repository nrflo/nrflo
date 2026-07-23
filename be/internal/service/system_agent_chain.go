package service

import (
	"fmt"

	"be/internal/model"
)

// ResolveAgentChain returns def's ordered model fallback chain — the
// SystemAgentDefinition adapter onto the shared resolveChain (Rule 6: one
// implementation, two thin adapters; see agent_def_chain.go for the
// AgentDefinition counterpart).
func (s *SystemAgentDefinitionService) ResolveAgentChain(def *model.SystemAgentDefinition) ([]AgentChainEntry, error) {
	if def == nil {
		return nil, fmt.Errorf("resolve agent chain: nil definition")
	}
	return resolveChain(s.pool, s.modelSvc, TierSpec{
		ID:              def.ID,
		Model:           def.Model,
		ExecutionMode:   def.ExecutionMode,
		ReasoningEffort: def.ReasoningEffort,
		Tier:            def.Tier,
	})
}
