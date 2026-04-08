package service

import (
	"sort"

	"be/internal/model"
)

// BuildSpawnerConfig converts DB models into spawner-compatible types.
// Shared by CLI agent spawn and server-side orchestrator.
// Phases are derived from agent definitions (layer field) instead of workflow JSON.
func BuildSpawnerConfig(dbWorkflows []*model.Workflow, dbAgentDefs []*model.AgentDefinition) (map[string]SpawnerWorkflowDef, map[string]SpawnerAgentConfig) {
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
				ID:    ad.ID,
				Agent: ad.ID,
				Layer: ad.Layer,
			})
		}
		// Sort by layer ASC, id ASC for deterministic ordering
		sort.Slice(phases, func(i, j int) bool {
			if phases[i].Layer != phases[j].Layer {
				return phases[i].Layer < phases[j].Layer
			}
			return phases[i].ID < phases[j].ID
		})

		scopeType := wf.ScopeType
		if scopeType == "" {
			scopeType = "ticket"
		}
		workflows[wf.ID] = SpawnerWorkflowDef{
			Description:           wf.Description,
			ScopeType:             scopeType,
			CloseTicketOnComplete: wf.CloseTicketOnComplete,
			Phases:                phases,
			Groups:                wf.GetGroups(),
		}
	}

	agents := make(map[string]SpawnerAgentConfig)
	for _, def := range dbAgentDefs {
		agents[def.ID] = SpawnerAgentConfig{
			Model:   def.Model,
			Timeout: def.Timeout,
			Tag:     def.Tag,
		}
	}

	return workflows, agents
}

// SpawnerWorkflowDef mirrors spawner.WorkflowDef for shared config building
type SpawnerWorkflowDef struct {
	Description           string            `json:"description"`
	ScopeType             string            `json:"scope_type"`
	CloseTicketOnComplete bool              `json:"close_ticket_on_complete"`
	Phases                []SpawnerPhaseDef `json:"phases"`
	Groups                []string          `json:"groups"`
}

// SpawnerPhaseDef mirrors spawner.PhaseDef for shared config building
type SpawnerPhaseDef struct {
	ID    string `json:"id"`
	Agent string `json:"agent"`
	Layer int    `json:"layer"`
}

// SpawnerAgentConfig mirrors spawner.AgentConfig for shared config building
type SpawnerAgentConfig struct {
	Model   string `json:"model"`
	Timeout int    `json:"timeout"`
	Tag     string `json:"tag"`
}

// parseWorkflowDefFromDB builds a WorkflowDef from agent definitions
func parseWorkflowDefFromDB(description string, agentDefs []*model.AgentDefinition) *WorkflowDef {
	var phases []PhaseDef
	for _, ad := range agentDefs {
		phases = append(phases, PhaseDef{
			ID:    ad.ID,
			Agent: ad.ID,
			Layer: ad.Layer,
		})
	}
	// Sort by layer ASC, id ASC for deterministic ordering
	sort.Slice(phases, func(i, j int) bool {
		if phases[i].Layer != phases[j].Layer {
			return phases[i].Layer < phases[j].Layer
		}
		return phases[i].ID < phases[j].ID
	})

	return &WorkflowDef{
		Description: description,
		Phases:      phases,
	}
}
