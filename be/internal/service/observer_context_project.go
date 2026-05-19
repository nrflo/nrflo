package service

import (
	"fmt"
	"strings"

	"be/internal/types"
)

// buildProjectDynamicContext renders workflow def list + recent project instances + sessions
// + project findings into a markdown bundle for a project-scope observer.
func buildProjectDynamicContext(s *ObserverService, projectID string) (string, error) {
	var b strings.Builder

	b.WriteString("## Project Context\n\n")
	b.WriteString(fmt.Sprintf("**Project:** %s\n\n", projectID))

	// Workflow definitions
	wfDefs, err := s.workflowSvc.ListWorkflowDefs(projectID)
	if err == nil && len(wfDefs) > 0 {
		b.WriteString("### Workflow Definitions\n\n")
		for id, def := range wfDefs {
			b.WriteString(fmt.Sprintf("- **%s**: %s (%d phases)\n", id, def.Description, len(def.Phases)))
		}
		b.WriteString("\n")
	}

	// Recent project workflow instances
	instances, err := s.workflowSvc.ListProjectWorkflowInstances(projectID)
	if err == nil && len(instances) > 0 {
		b.WriteString("### Recent Project Instances\n\n")
		limit := 5
		for i, wi := range instances {
			if i >= limit {
				break
			}
			b.WriteString(fmt.Sprintf("- **%s** workflow=%s status=%s\n", wi.ID, wi.WorkflowID, wi.Status))
		}
		b.WriteString("\n")
	}

	// Recent sessions
	sessions, err := s.agentSvc.GetProjectSessions(projectID, "")
	if err == nil && len(sessions) > 0 {
		b.WriteString("### Recent Sessions\n\n")
		limit := 10
		for i, sess := range sessions {
			if i >= limit {
				break
			}
			b.WriteString(fmt.Sprintf("- %s workflow=%s phase=%s status=%s\n", sess.ID[:8], sess.Workflow, sess.Phase, sess.Status))
		}
		b.WriteString("\n")
	}

	// Project findings
	findings, err := s.projectFindingsSvc.Get(projectID, &types.ProjectFindingsGetRequest{})
	if err == nil && findings != nil {
		if fm, ok := findings.(map[string]interface{}); ok && len(fm) > 0 {
			b.WriteString("### Project Findings\n\n")
			for k, v := range fm {
				b.WriteString(fmt.Sprintf("- **%s**: %v\n", k, v))
			}
			b.WriteString("\n")
		}
	}

	return b.String(), nil
}
