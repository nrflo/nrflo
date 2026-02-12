package service

import (
	"encoding/json"
	"fmt"
	"sort"
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
