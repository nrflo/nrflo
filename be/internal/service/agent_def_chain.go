package service

import (
	"fmt"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
)

// ResolveDefChain is a package-level helper for callers (workflow_config.go,
// workflow_instance_nodes.go, orchestrator/planner.go) that resolve a
// workflow-local agent_definitions chain without holding an
// AgentDefinitionService instance. clk is accepted for symmetry with other
// pool-based helpers but unused — chain resolution reads tier_models/models
// directly.
func ResolveDefChain(pool *db.Pool, _ clock.Clock, modelSvc *ModelService, def *model.AgentDefinition) ([]AgentChainEntry, error) {
	if def == nil {
		return nil, fmt.Errorf("resolve agent chain: nil definition")
	}
	return resolveChain(pool, modelSvc, TierSpec{
		ID:              def.ID,
		Model:           def.Model,
		ExecutionMode:   def.ExecutionMode,
		ReasoningEffort: def.ReasoningEffort,
		Tier:            def.Tier,
	})
}
