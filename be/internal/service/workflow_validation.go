package service

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// validateLayerConfig validates layer-based phase configuration rules:
// - layer must be >= 0
// - no "parallel" field allowed in raw JSON
// - fan-in: if layer N has >1 agent, the next non-empty layer must have exactly 1 agent
func validateLayerConfig(phases []PhaseDef, rawPhases []json.RawMessage) error {
	// Check for rejected "parallel" field in raw JSON
	for _, raw := range rawPhases {
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(raw, &obj); err == nil {
			if _, hasParallel := obj["parallel"]; hasParallel {
				return fmt.Errorf("'parallel' field is no longer supported. Migrate to layer-based parallelism: use {\"agent\": \"name\", \"layer\": N}")
			}
		}
	}

	// Validate layer values
	for _, p := range phases {
		if p.Layer < 0 {
			return fmt.Errorf("agent '%s': layer must be >= 0, got %d", p.Agent, p.Layer)
		}
	}

	// Group agents by layer for fan-in validation
	layerCounts := make(map[int]int)
	for _, p := range phases {
		layerCounts[p.Layer]++
	}

	// Get sorted layer numbers
	var layers []int
	for l := range layerCounts {
		layers = append(layers, l)
	}
	sort.Ints(layers)

	// Fan-in check: if layer N has >1 agent, next non-empty layer must have exactly 1 agent
	for i, layer := range layers {
		if layerCounts[layer] > 1 && i+1 < len(layers) {
			nextLayer := layers[i+1]
			if layerCounts[nextLayer] != 1 {
				return fmt.Errorf("fan-in violation: layer %d has %d agents, so layer %d must have exactly 1 agent (has %d). Parallel layers must converge to a single downstream agent",
					layer, layerCounts[layer], nextLayer, layerCounts[nextLayer])
			}
		}
	}

	return nil
}

// rejectStringPhaseEntry checks if a raw JSON value is a string (legacy format)
// and returns an error with migration hint if so.
func rejectStringPhaseEntry(raw json.RawMessage) error {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return fmt.Errorf("string phase entry '%s' is no longer supported. Migrate to object format: {\"agent\": \"%s\", \"layer\": 0}", s, s)
	}
	return nil
}

// ticketTemplateVars are template variables that require ticket context
var ticketTemplateVars = []string{"${TICKET_ID}", "${TICKET_TITLE}", "${TICKET_DESCRIPTION}"}

// ValidateProjectScope checks that agent prompts don't use ticket-specific template variables
// when scope_type is "project". Loads agent definitions from DB to check their prompts.
func ValidateProjectScope(pool interface{ QueryRow(string, ...interface{}) interface{ Scan(...interface{}) error } }, projectID, workflowID string, phases []PhaseDef) error {
	for _, phase := range phases {
		var prompt string
		err := pool.QueryRow(`
			SELECT prompt FROM agent_definitions
			WHERE LOWER(project_id) = LOWER(?) AND LOWER(workflow_id) = LOWER(?) AND LOWER(id) = LOWER(?)`,
			projectID, workflowID, phase.Agent).Scan(&prompt)
		if err != nil {
			continue // Agent definition may not exist yet
		}
		for _, v := range ticketTemplateVars {
			if strings.Contains(prompt, v) {
				return fmt.Errorf("agent '%s' uses %s in its prompt, which is not available for project-scoped workflows", phase.Agent, v)
			}
		}
	}
	return nil
}

// ValidateScopeType validates that scope_type is a valid value
func ValidateScopeType(scopeType string) error {
	if scopeType != "" && scopeType != "ticket" && scopeType != "project" {
		return fmt.Errorf("scope_type must be 'ticket' or 'project', got '%s'", scopeType)
	}
	return nil
}
