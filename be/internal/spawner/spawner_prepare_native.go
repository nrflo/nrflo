package spawner

import "be/internal/model"

// resolveReasoningEffort resolves the winning reasoning_effort override with
// precedence def-level override > AgentConfig-carried override > the model
// row's own effort (rowEffort). The AgentConfig fallback exists because a
// global workflow's agent def is invisible to the spawner (loadAgentDefinition
// has no __global__ fallback — see orchestrator/plan_boundary.go), so a
// materialized plan node's override must also travel via
// s.config.Agents[agentType].ReasoningEffort. Mirrors the executionMode
// fallback shape (agentDef != nil check, else config.Agents lookup).
func (s *Spawner) resolveReasoningEffort(agentDef *model.AgentDefinition, agentType, rowEffort string) string {
	if agentDef != nil && agentDef.ReasoningEffort != nil {
		return *agentDef.ReasoningEffort
	}
	if agentDef == nil {
		if agentCfg, ok := s.config.Agents[agentType]; ok && agentCfg.ReasoningEffort != nil {
			return *agentCfg.ReasoningEffort
		}
	}
	return rowEffort
}

// nativeSpawnFields resolves the per-def native tool restriction for a CLI
// spawn: native_tools applies only to claude (--tools allowlist), sandbox
// only to codex (thread/start sandbox). A nil def (global workflow defs are
// invisible to loadAgentDefinition) leaves both empty = unrestricted.
func nativeSpawnFields(agentDef *model.AgentDefinition, adapterName string) (nativeToolsCSV, sandbox string) {
	if agentDef == nil {
		return "", ""
	}
	switch adapterName {
	case "claude":
		return agentDef.NativeTools, ""
	case "codex":
		return "", agentDef.Sandbox
	}
	return "", ""
}
