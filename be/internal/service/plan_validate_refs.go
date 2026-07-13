package service

import (
	"fmt"
	"regexp"
	"strings"

	"be/internal/clock"
	"be/internal/db"
)

// planNodeFindingsPattern mirrors spawner/template.go's nodeFindingsPattern so
// the validator checks references using the exact same syntax the spawner
// expands at runtime.
var planNodeFindingsPattern = regexp.MustCompile(`#\{NODE_FINDINGS:([^:}]+)(?::([^}]*))?\}`)

// validatePlanRefsAndTemplates checks that every #{NODE_FINDINGS:...} reference
// in a node's instructions resolves to a node declared in a strictly earlier
// layer, and that every node's template exists in the workflow's template
// library and its model is currently enabled. Returns aggregated problem
// strings (nil when everything resolves).
func validatePlanRefsAndTemplates(pool *db.Pool, projectID, workflowID string, m PlanManifest) []string {
	var problems []string

	// node id -> layer index. Built independent of earlier validation so a
	// malformed plan still gets a complete error report in one pass.
	nodeLayer := make(map[string]int)
	for i, layer := range m.Layers {
		for _, node := range layer.Nodes {
			if _, dup := nodeLayer[node.ID]; !dup {
				nodeLayer[node.ID] = i
			}
		}
	}

	for i, layer := range m.Layers {
		for _, node := range layer.Nodes {
			for _, match := range planNodeFindingsPattern.FindAllStringSubmatch(node.Instructions, -1) {
				target := match[1]
				targetLayer, ok := nodeLayer[target]
				if !ok {
					problems = append(problems, fmt.Sprintf(
						"node %q references #{NODE_FINDINGS:%s}, which is not a node declared anywhere in this plan", node.ID, target))
					continue
				}
				if targetLayer >= i {
					problems = append(problems, fmt.Sprintf(
						"node %q (layer %d) references #{NODE_FINDINGS:%s} (layer %d): references must target a strictly earlier layer",
						node.ID, i, target, targetLayer))
				}
			}
		}
	}

	templates, err := AllowedTemplates(pool, projectID, workflowID)
	if err != nil {
		problems = append(problems, fmt.Sprintf("failed to load template library: %v", err))
		return problems
	}
	byName := make(map[string]PlanTemplate, len(templates))
	names := make([]string, 0, len(templates))
	for _, t := range templates {
		byName[t.ID] = t
		names = append(names, t.ID)
	}

	referenced := make(map[string]PlanTemplate)
	for _, layer := range m.Layers {
		for _, node := range layer.Nodes {
			if node.Template == "" {
				continue
			}
			t, ok := byName[node.Template]
			if !ok {
				problems = append(problems, fmt.Sprintf(
					"node %q references unknown template %q; available templates: %s", node.ID, node.Template, strings.Join(names, ", ")))
				continue
			}
			referenced[t.ID] = t
		}
	}

	if len(referenced) > 0 {
		refList := make([]PlanTemplate, 0, len(referenced))
		for _, t := range referenced {
			refList = append(refList, t)
		}
		if err := ValidateTemplatesEnabled(pool, clock.Real(), refList); err != nil {
			problems = append(problems, err.Error())
		}
	}

	return problems
}
