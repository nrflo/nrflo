package service

import (
	"fmt"
	"regexp"
	"strings"

	"be/internal/db"
)

// planNodeIDPattern matches the allowed node id shape: lowercase alnum, dash,
// underscore, starting with an alnum (so it can never collide with the
// `_`-prefixed reserved node ids used for internal transient sessions, e.g.
// _planner/_consult — see service/workflow_response.go transientAgentTypeExclusion).
var planNodeIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

// ValidatePlanManifest is the authoritative semantic validator for a plan
// manifest. It aggregates every rule violation into one multi-line error so a
// planner agent can fix everything in a single retry (the emit_findings
// philosophy — see findings_emit.go). The caller (FindingsService.Emit)
// appends the manifest example on failure.
func ValidatePlanManifest(pool *db.Pool, projectID, workflowID string, m PlanManifest) error {
	limits := LoadPlanLimits(pool, projectID)
	var problems []string

	if m.Version != 1 {
		problems = append(problems, fmt.Sprintf("version must be 1 (got %d)", m.Version))
	}
	if strings.TrimSpace(m.Goal) == "" {
		problems = append(problems, "goal is required")
	}
	if len(m.Layers) == 0 {
		problems = append(problems, "at least one layer is required")
	}
	if len(m.Layers) > limits.MaxLayers {
		problems = append(problems, fmt.Sprintf("too many layers: %d (max %d)", len(m.Layers), limits.MaxLayers))
	}
	if len(m.Questions) > limits.MaxQuestions {
		problems = append(problems, fmt.Sprintf("too many questions: %d (max %d)", len(m.Questions), limits.MaxQuestions))
	}

	seenIDs := make(map[string]bool)
	totalNodes := 0
	for i, layer := range m.Layers {
		if layer.Layer != i {
			problems = append(problems, fmt.Sprintf(
				"layers must be dense and 0-indexed: layer at position %d has layer=%d (expected %d)", i, layer.Layer, i))
		}
		if len(layer.Nodes) == 0 {
			problems = append(problems, fmt.Sprintf("layer %d must have at least one node", layer.Layer))
		}
		if err := ValidateLayerPolicy(layer.Policy, len(layer.Nodes)); err != nil {
			problems = append(problems, fmt.Sprintf("layer %d: invalid policy %q: %v", layer.Layer, layer.Policy, err))
		}
		if i == len(m.Layers)-1 && len(layer.Nodes) != 1 {
			problems = append(problems, fmt.Sprintf(
				"the final layer (%d) must have exactly one node (the result-carrying node); got %d", layer.Layer, len(layer.Nodes)))
		}
		for _, node := range layer.Nodes {
			totalNodes++
			if !planNodeIDPattern.MatchString(node.ID) {
				problems = append(problems, fmt.Sprintf("node id %q is invalid: must match ^[a-z0-9][a-z0-9_-]{0,63}$", node.ID))
			}
			if strings.HasPrefix(node.ID, "_") {
				problems = append(problems, fmt.Sprintf("node id %q must not start with '_' (reserved for internal node ids)", node.ID))
			}
			if seenIDs[node.ID] {
				problems = append(problems, fmt.Sprintf("duplicate node id %q", node.ID))
			}
			seenIDs[node.ID] = true
			if strings.TrimSpace(node.Instructions) == "" {
				problems = append(problems, fmt.Sprintf("node %q: instructions are required", node.ID))
			}
			if len(node.Instructions) > limits.MaxInstructionBytes {
				problems = append(problems, fmt.Sprintf("node %q: instructions exceed %d bytes (max %d)", node.ID, len(node.Instructions), limits.MaxInstructionBytes))
			}
			if strings.TrimSpace(node.Template) == "" {
				problems = append(problems, fmt.Sprintf("node %q: template is required", node.ID))
			}
		}
	}
	if totalNodes > limits.MaxNodes {
		problems = append(problems, fmt.Sprintf("too many nodes: %d (max %d)", totalNodes, limits.MaxNodes))
	}

	problems = append(problems, validatePlanRefsAndTemplates(pool, projectID, workflowID, m)...)

	if len(problems) == 0 {
		return nil
	}
	return fmt.Errorf("%s", strings.Join(problems, "\n"))
}
