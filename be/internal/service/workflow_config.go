package service

import (
	"context"
	"sort"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/logger"
	"be/internal/model"
)

// BuildSpawnerConfig converts DB models into spawner-compatible types.
// Shared by CLI agent spawn and server-side orchestrator. Phases are derived
// from agent definitions (layer field) instead of workflow JSON. pool/clk are
// used to resolve a tier fallback chain for any def with Model=="" &&
// Tier!=nil (Chain resolution failures never fail the whole call — they log
// a warning and fall back to the def's raw model, same style as
// LoadMaterializedAgentConfigs).
func BuildSpawnerConfig(pool *db.Pool, clk clock.Clock, dbWorkflows []*model.Workflow, dbAgentDefs []*model.AgentDefinition) (map[string]SpawnerWorkflowDef, map[string]SpawnerAgentConfig) {
	// Group agent definitions by workflow ID
	agentsByWorkflow := make(map[string][]*model.AgentDefinition)
	for _, ad := range dbAgentDefs {
		agentsByWorkflow[ad.WorkflowID] = append(agentsByWorkflow[ad.WorkflowID], ad)
	}

	workflows := make(map[string]SpawnerWorkflowDef)
	for _, wf := range dbWorkflows {
		var phases []SpawnerPhaseDef
		for _, ad := range agentsByWorkflow[wf.ID] {
			phases = append(phases, SpawnerPhaseDef{
				NodeID: ad.ID,
				Agent:  ad.ID,
				Layer:  ad.Layer,
			})
		}
		// Sort by layer ASC, id ASC for deterministic ordering
		sort.Slice(phases, func(i, j int) bool {
			if phases[i].Layer != phases[j].Layer {
				return phases[i].Layer < phases[j].Layer
			}
			return phases[i].NodeID < phases[j].NodeID
		})

		scopeType := wf.ScopeType
		if scopeType == "" {
			scopeType = "ticket"
		}
		workflows[wf.ID] = SpawnerWorkflowDef{
			Description:             wf.Description,
			ScopeType:               scopeType,
			CloseTicketOnComplete:   wf.CloseTicketOnComplete,
			FinalizeSuccessCommand:  wf.FinalizeSuccessCommand,
			FinalizeSuccessScriptID: wf.FinalizeSuccessScriptID,
			FinalizeFailureCommand:  wf.FinalizeFailureCommand,
			FinalizeFailureScriptID: wf.FinalizeFailureScriptID,
			PauseEventCommand:       wf.PauseEventCommand,
			PauseEventScriptID:      wf.PauseEventScriptID,
			Phases:                  phases,
			Groups:                  wf.GetGroups(),
		}
	}

	modelSvc := NewModelService(pool, clk)
	agents := make(map[string]SpawnerAgentConfig)
	for _, def := range dbAgentDefs {
		cfg := SpawnerAgentConfig{
			Model:           def.Model,
			Timeout:         def.Timeout,
			Tag:             def.Tag,
			ReasoningEffort: def.ReasoningEffort,
		}
		if def.Model == "" && def.Tier != nil {
			if chain, err := ResolveDefChain(pool, clk, modelSvc, def); err != nil {
				logger.Warn(context.Background(), "BuildSpawnerConfig: resolve tier chain failed, falling back to raw model", "agent", def.ID, "err", err)
			} else if len(chain) > 0 {
				primary := chain[0]
				effort := primary.ReasoningEffort
				cfg.Model = primary.ModelID
				cfg.ReasoningEffort = &effort
				cfg.Chain = chain
			}
		}
		agents[def.ID] = cfg
	}

	return workflows, agents
}

// SpawnerWorkflowDef mirrors spawner.WorkflowDef for shared config building
type SpawnerWorkflowDef struct {
	Description             string            `json:"description"`
	ScopeType               string            `json:"scope_type"`
	CloseTicketOnComplete   bool              `json:"close_ticket_on_complete"`
	FinalizeSuccessCommand  string            `json:"finalize_success_command"`
	FinalizeSuccessScriptID string            `json:"finalize_success_script_id"`
	FinalizeFailureCommand  string            `json:"finalize_failure_command"`
	FinalizeFailureScriptID string            `json:"finalize_failure_script_id"`
	PauseEventCommand       string            `json:"pause_event_command"`
	PauseEventScriptID      string            `json:"pause_event_script_id"`
	Phases                  []SpawnerPhaseDef `json:"phases"`
	Groups                  []string          `json:"groups"`
	LayerPolicies           map[int]string    `json:"layer_policies,omitempty"`
}

// SpawnerPhaseDef mirrors spawner.PhaseDef for shared config building.
// Instructions is set only for materialized plan nodes (DYNWF-5); empty for
// static agent_definitions-derived phases.
type SpawnerPhaseDef struct {
	NodeID       string `json:"id"`
	Agent        string `json:"agent"`
	Layer        int    `json:"layer"`
	Instructions string `json:"instructions,omitempty"`
}

// SpawnerAgentConfig mirrors spawner.AgentConfig for shared config building
type SpawnerAgentConfig struct {
	Model           string  `json:"model"`
	Timeout         int     `json:"timeout"`
	Tag             string  `json:"tag"`
	ReasoningEffort *string `json:"reasoning_effort,omitempty"`
	// Chain is the resolved tier fallback chain for defs with Model=="" &&
	// Tier!=nil (index 0 = primary, already reflected in Model/ReasoningEffort
	// above); nil for defs with an explicit model override.
	Chain []AgentChainEntry `json:"-"`
}

// parseWorkflowDefFromDB builds a WorkflowDef from agent definitions
func parseWorkflowDefFromDB(description string, agentDefs []*model.AgentDefinition) *WorkflowDef {
	var phases []PhaseDef
	for _, ad := range agentDefs {
		phases = append(phases, PhaseDef{
			NodeID: ad.ID,
			Agent:  ad.ID,
			Layer:  ad.Layer,
		})
	}
	// Sort by layer ASC, id ASC for deterministic ordering
	sort.Slice(phases, func(i, j int) bool {
		if phases[i].Layer != phases[j].Layer {
			return phases[i].Layer < phases[j].Layer
		}
		return phases[i].NodeID < phases[j].NodeID
	})

	return &WorkflowDef{
		Description: description,
		Phases:      phases,
	}
}
