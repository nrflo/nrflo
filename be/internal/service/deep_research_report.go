package service

import (
	"encoding/json"
	"fmt"
)

// FindReportFinding locates the synthesize agent's "report" value in a v4
// workflow_findings map. BuildCombinedFindings keys that map by
// "<agent_type>[:<model_id>]" → {findingKey: value} (so the report is at
// combined["synthesize:claude:opus-4-8"]["report"]), so search the per-agent
// groups. A flat {report: ...} shape is also accepted.
func FindReportFinding(combined map[string]any) (any, bool) {
	if combined == nil {
		return nil, false
	}
	if r, ok := combined["report"]; ok {
		return r, true
	}
	for _, group := range combined {
		if g, ok := group.(map[string]any); ok {
			if r, ok := g["report"]; ok {
				return r, true
			}
		}
	}
	return nil, false
}

// ExtractReport pulls the `report` finding out of a terminal v4 state's
// workflow_findings map (or a flat {report:...} map), rendering non-string
// values as indented JSON.
func ExtractReport(combined map[string]any, instanceID string) (string, error) {
	report, ok := FindReportFinding(combined)
	if !ok {
		return "", fmt.Errorf("deep-research run %s completed but emitted no 'report' finding", instanceID)
	}
	if s, isStr := report.(string); isStr {
		return s, nil
	}
	b, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}
