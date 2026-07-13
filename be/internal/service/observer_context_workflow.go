package service

import (
	"fmt"
	"strings"

	"be/internal/types"
)

// buildWorkflowDynamicContext renders workflow def + recent instances + sessions + findings
// into a markdown bundle for a workflow-scope observer.
func buildWorkflowDynamicContext(s *ObserverService, projectID, workflowID string) (string, error) {
	var b strings.Builder

	b.WriteString("## Workflow Context\n\n")

	// Workflow definition
	wfDef, err := s.workflowSvc.GetWorkflowDef(projectID, workflowID)
	if err != nil {
		fmt.Fprintf(&b, "_Workflow %q not found._\n\n", workflowID)
	} else {
		fmt.Fprintf(&b, "### Workflow: %s\n\n", workflowID)
		fmt.Fprintf(&b, "**Description:** %s\n\n", wfDef.Description)
		if len(wfDef.Phases) > 0 {
			b.WriteString("**Phases:**\n\n")
			for _, p := range wfDef.Phases {
				fmt.Fprintf(&b, "- layer %d: %s (%s)\n", p.Layer, p.NodeID, p.Agent)
			}
			b.WriteString("\n")
		}
	}

	// Recent workflow instances
	instances, err := s.workflowSvc.ListWorkflowInstances(projectID, "")
	if err == nil && len(instances) > 0 {
		b.WriteString("### Recent Instances\n\n")
		limit := 5
		for i, wi := range instances {
			if i >= limit {
				break
			}
			if wi.WorkflowID != workflowID {
				continue
			}
			fmt.Fprintf(&b, "- **%s** status=%s created=%s\n", wi.ID, wi.Status, wi.CreatedAt.Format("2006-01-02T15:04:05Z"))

			// Per-instance findings
			findings, fErr := s.findingsSvc.Get(&types.FindingsGetRequest{InstanceID: wi.ID})
			if fErr == nil && findings != nil {
				if fm, ok := findings.(map[string]interface{}); ok && len(fm) > 0 {
					b.WriteString("  Findings: ")
					keys := make([]string, 0, len(fm))
					for k := range fm {
						keys = append(keys, k)
					}
					b.WriteString(strings.Join(keys, ", "))
					b.WriteString("\n")
				}
			}
		}
		b.WriteString("\n")
	}

	// Recent sessions for this workflow
	sessions, err := s.agentSvc.GetProjectSessions(projectID, "")
	if err == nil && len(sessions) > 0 {
		b.WriteString("### Recent Sessions\n\n")
		limit := 10
		count := 0
		for _, sess := range sessions {
			if count >= limit {
				break
			}
			if sess.Workflow != workflowID {
				continue
			}
			fmt.Fprintf(&b, "- %s phase=%s status=%s\n", sess.ID[:8], sess.Phase, sess.Status)
			count++
		}
		b.WriteString("\n")
	}

	return b.String(), nil
}
