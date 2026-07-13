package service

import (
	"be/internal/clock"
	"be/internal/db"
	"be/internal/repo"
)

// IsPlanDriven reports whether a workflow definition has at least one
// fanout_template agent definition — i.e., whether the engine should check
// for a plan boundary once its static executable layers are exhausted.
func IsPlanDriven(pool *db.Pool, defProjectID, workflowID string) (bool, error) {
	templates, err := AllowedTemplates(pool, defProjectID, workflowID)
	if err != nil {
		return false, err
	}
	return len(templates) > 0, nil
}

// LoadInstanceNodePhases loads a workflow instance's materialized plan nodes
// as spawner phase defs (NodeID=node_id, Agent=agent_type, Layer,
// Instructions) plus its materialized layer -> pass_policy map. Both are
// empty when nothing has been materialized yet.
func LoadInstanceNodePhases(pool *db.Pool, clk clock.Clock, instanceID string) ([]SpawnerPhaseDef, map[int]string, error) {
	nodeRepo := repo.NewInstanceNodeRepo(pool, clk)
	nodes, err := nodeRepo.List(instanceID)
	if err != nil {
		return nil, nil, err
	}
	phases := make([]SpawnerPhaseDef, len(nodes))
	for i, n := range nodes {
		phases[i] = SpawnerPhaseDef{NodeID: n.NodeID, Agent: n.AgentType, Layer: n.Layer, Instructions: n.Instructions}
	}
	policies, err := nodeRepo.ListLayerPolicies(instanceID)
	if err != nil {
		return nil, nil, err
	}
	return phases, policies, nil
}

// EffectivePhases is the single source of truth for a run's execution graph:
// static agent_definitions-derived phases followed by any materialized plan
// nodes. Called by runLoop, ContinueWorkflow, retryFailed, and buildV4State so
// none of them re-derive the graph independently.
func EffectivePhases(static, materialized []SpawnerPhaseDef) []SpawnerPhaseDef {
	if len(materialized) == 0 {
		return static
	}
	out := make([]SpawnerPhaseDef, 0, len(static)+len(materialized))
	out = append(out, static...)
	out = append(out, materialized...)
	return out
}

// LoadMaterializedAgentConfigs resolves model/timeout/tag for each distinct
// fanout_template referenced by materialized phases. Static phases resolve
// their agent config from the workflow's pre-loaded agents map (built from
// ListExecutable, which excludes fanout_template defs); materialized nodes
// bind to a fanout_template def instead, so the spawner's agents map must be
// extended with these or a materialized node spawns with the hardcoded
// default model instead of its configured template.
func LoadMaterializedAgentConfigs(pool *db.Pool, clk clock.Clock, defProjectID, workflowID string, materialized []SpawnerPhaseDef) map[string]SpawnerAgentConfig {
	if len(materialized) == 0 {
		return nil
	}
	adRepo := repo.NewAgentDefinitionRepo(pool, clk)
	out := make(map[string]SpawnerAgentConfig)
	for _, p := range materialized {
		if _, ok := out[p.Agent]; ok {
			continue
		}
		def, err := adRepo.Get(defProjectID, workflowID, p.Agent)
		if err != nil {
			continue // template removed since materialization; spawn surfaces a clear "not found" error
		}
		out[p.Agent] = SpawnerAgentConfig{Model: def.Model, Timeout: def.Timeout, Tag: def.Tag, ReasoningEffort: def.ReasoningEffort}
	}
	return out
}
