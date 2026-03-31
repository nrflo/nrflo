package service

import (
	"encoding/json"
	"fmt"
	"sort"

	"be/internal/model"
)

// --- Phase Parsing Helpers ---

// parsePhaseDefs parses object-format phase definitions with required layer field.
// String-only entries and parallel field are rejected.
func parsePhaseDefs(rawPhases []json.RawMessage) ([]PhaseDef, error) {
	var phases []PhaseDef
	for _, raw := range rawPhases {
		// Reject string-only entries
		if err := rejectStringPhaseEntry(raw); err != nil {
			return nil, err
		}
		// Parse object format
		var phase struct {
			Agent string `json:"agent"`
			Layer int    `json:"layer"`
			Order int    `json:"order,omitempty"`
		}
		if err := json.Unmarshal(raw, &phase); err != nil || phase.Agent == "" {
			return nil, fmt.Errorf("invalid phase: %s (must be object with 'agent' and 'layer' fields)", string(raw))
		}
		phases = append(phases, PhaseDef{
			ID:    phase.Agent,
			Agent: phase.Agent,
			Layer: phase.Layer,
			Order: phase.Order,
		})
	}
	// Validate layer config and reject parallel field
	if err := validateLayerConfig(phases, rawPhases); err != nil {
		return nil, err
	}
	return phases, nil
}

// normalizePhasesJSON validates and normalizes phases JSON input.
// Requires object format with layer field. Rejects string entries and parallel field.
func normalizePhasesJSON(raw json.RawMessage) (json.RawMessage, error) {
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, fmt.Errorf("phases must be a JSON array: %w", err)
	}

	var normalized []interface{}
	for _, item := range items {
		// Reject string entries
		if err := rejectStringPhaseEntry(item); err != nil {
			return nil, err
		}
		// Validate object
		var obj map[string]interface{}
		if err := json.Unmarshal(item, &obj); err != nil {
			return nil, fmt.Errorf("invalid phase entry: %s", string(item))
		}
		if _, ok := obj["agent"]; !ok {
			return nil, fmt.Errorf("phase object must have 'agent' field")
		}
		normalized = append(normalized, obj)
	}

	// Full validation via parsePhaseDefs (layer, fan-in, parallel rejection)
	rawMessages := make([]json.RawMessage, len(items))
	copy(rawMessages, items)
	if _, err := parsePhaseDefs(rawMessages); err != nil {
		return nil, err
	}

	return json.Marshal(normalized)
}

// BuildSpawnerConfig converts DB models into spawner-compatible types.
// Shared by CLI agent spawn and server-side orchestrator.
func BuildSpawnerConfig(dbWorkflows []*model.Workflow, dbAgentDefs []*model.AgentDefinition) (map[string]SpawnerWorkflowDef, map[string]SpawnerAgentConfig) {
	workflows := make(map[string]SpawnerWorkflowDef)
	for _, wf := range dbWorkflows {
		var rawPhases []json.RawMessage
		if err := json.Unmarshal([]byte(wf.Phases), &rawPhases); err != nil {
			continue
		}

		var phases []SpawnerPhaseDef
		for _, raw := range rawPhases {
			var pd SpawnerPhaseDef
			if err := json.Unmarshal(raw, &pd); err == nil && pd.Agent != "" {
				if pd.ID == "" {
					pd.ID = pd.Agent
				}
				phases = append(phases, pd)
			}
		}

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

// parseWorkflowDefFromDB parses a WorkflowDef from database fields
func parseWorkflowDefFromDB(description, phasesStr string) (*WorkflowDef, error) {
	var rawPhases []json.RawMessage
	if err := json.Unmarshal([]byte(phasesStr), &rawPhases); err != nil {
		return nil, fmt.Errorf("invalid phases JSON: %w", err)
	}

	phases, err := parsePhaseDefs(rawPhases)
	if err != nil {
		return nil, err
	}

	// Sort phases by Order field
	sort.Slice(phases, func(i, j int) bool {
		return phases[i].Order < phases[j].Order
	})

	return &WorkflowDef{
		Description: description,
		Phases:      phases,
		RawPhases:   rawPhases,
	}, nil
}
