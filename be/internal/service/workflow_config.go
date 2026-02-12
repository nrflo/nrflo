package service

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"

	"be/internal/model"
)

// --- Phase Parsing Helpers ---

// parsePhaseDefs parses mixed-format phase definitions (string or object)
func parsePhaseDefs(rawPhases []json.RawMessage) ([]PhaseDef, error) {
	var phases []PhaseDef
	for _, raw := range rawPhases {
		// Try string first (simple format: just agent name)
		var agentName string
		if err := json.Unmarshal(raw, &agentName); err == nil {
			phases = append(phases, PhaseDef{ID: agentName, Agent: agentName})
			continue
		}
		// Try object format
		var phase struct {
			Agent    string   `json:"agent"`
			Order    int      `json:"order,omitempty"`
			SkipFor  []string `json:"skip_for,omitempty"`
			Parallel *struct {
				Enabled bool     `json:"enabled"`
				Models  []string `json:"models"`
			} `json:"parallel,omitempty"`
		}
		if err := json.Unmarshal(raw, &phase); err == nil && phase.Agent != "" {
			p := PhaseDef{
				ID:      phase.Agent,
				Agent:   phase.Agent,
				Order:   phase.Order,
				SkipFor: phase.SkipFor,
			}
			if phase.Parallel != nil {
				p.Parallel = &struct {
					Enabled bool     `json:"enabled"`
					Models  []string `json:"models"`
				}{
					Enabled: phase.Parallel.Enabled,
					Models:  phase.Parallel.Models,
				}
			}
			phases = append(phases, p)
			continue
		}
		return nil, fmt.Errorf("invalid phase: %s", string(raw))
	}
	return phases, nil
}

// normalizePhasesJSON validates and normalizes phases JSON input.
// Accepts mixed string/object format and normalizes strings to {"agent":"name"}.
func normalizePhasesJSON(raw json.RawMessage) (json.RawMessage, error) {
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, fmt.Errorf("phases must be a JSON array: %w", err)
	}

	var normalized []interface{}
	for _, item := range items {
		// Try string
		var agentName string
		if err := json.Unmarshal(item, &agentName); err == nil {
			normalized = append(normalized, map[string]interface{}{"agent": agentName})
			continue
		}
		// Try object
		var obj map[string]interface{}
		if err := json.Unmarshal(item, &obj); err == nil {
			if _, ok := obj["agent"]; !ok {
				return nil, fmt.Errorf("phase object must have 'agent' field")
			}
			normalized = append(normalized, obj)
			continue
		}
		return nil, fmt.Errorf("invalid phase entry: %s", string(item))
	}

	// Also validate via parsePhaseDefs
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
			var agentName string
			if err := json.Unmarshal(raw, &agentName); err == nil {
				phases = append(phases, SpawnerPhaseDef{ID: agentName, Agent: agentName})
				continue
			}
			var pd SpawnerPhaseDef
			if err := json.Unmarshal(raw, &pd); err == nil && pd.Agent != "" {
				if pd.ID == "" {
					pd.ID = pd.Agent
				}
				phases = append(phases, pd)
			}
		}

		var categories []string
		if wf.Categories.Valid && wf.Categories.String != "" {
			_ = json.Unmarshal([]byte(wf.Categories.String), &categories)
		}

		workflows[wf.ID] = SpawnerWorkflowDef{
			Description: wf.Description,
			Categories:  categories,
			Phases:      phases,
		}
	}

	agents := make(map[string]SpawnerAgentConfig)
	for _, def := range dbAgentDefs {
		agents[def.ID] = SpawnerAgentConfig{
			Model:   def.Model,
			Timeout: def.Timeout,
		}
	}

	return workflows, agents
}

// SpawnerWorkflowDef mirrors spawner.WorkflowDef for shared config building
type SpawnerWorkflowDef struct {
	Description string           `json:"description"`
	Categories  []string         `json:"categories"`
	Phases      []SpawnerPhaseDef `json:"phases"`
}

// SpawnerPhaseDef mirrors spawner.PhaseDef for shared config building
type SpawnerPhaseDef struct {
	ID       string   `json:"id"`
	Agent    string   `json:"agent"`
	SkipFor  []string `json:"skip_for,omitempty"`
	Parallel *struct {
		Enabled bool     `json:"enabled"`
		Models  []string `json:"models"`
	} `json:"parallel,omitempty"`
}

// SpawnerAgentConfig mirrors spawner.AgentConfig for shared config building
type SpawnerAgentConfig struct {
	Model   string `json:"model"`
	Timeout int    `json:"timeout"`
}

// parseWorkflowDefFromDB parses a WorkflowDef from database fields
func parseWorkflowDefFromDB(description string, categoriesStr sql.NullString, phasesStr string) (*WorkflowDef, error) {
	var categories []string
	if categoriesStr.Valid && categoriesStr.String != "" {
		_ = json.Unmarshal([]byte(categoriesStr.String), &categories)
	}

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
		Categories:  categories,
		Phases:      phases,
		RawPhases:   rawPhases,
	}, nil
}
