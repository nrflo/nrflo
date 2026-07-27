package spawner

import "be/internal/model"

// resolveReasoningEffort resolves the winning reasoning_effort override with
// precedence def-level override > AgentConfig-carried override > the model
// row's own effort (rowEffort). The AgentConfig step is not a nil-def fallback:
// a tier-based def (model empty, tier set) leaves ReasoningEffort nil and
// carries the chain-resolved effort only in s.config.Agents[agentType], so it
// must be consulted whenever the def itself states no override.
func (s *Spawner) resolveReasoningEffort(agentDef *model.AgentDefinition, agentType, rowEffort string) string {
	if agentDef != nil && agentDef.ReasoningEffort != nil {
		return *agentDef.ReasoningEffort
	}
	if agentCfg, ok := s.config.Agents[agentType]; ok && agentCfg.ReasoningEffort != nil {
		return *agentCfg.ReasoningEffort
	}
	return rowEffort
}

// nativeSpawnFields resolves the per-def native tool restriction for a CLI
// spawn: native_tools applies only to claude (--tools allowlist), sandbox
// only to codex (thread/start sandbox). A nil def leaves both empty =
// unrestricted.
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
