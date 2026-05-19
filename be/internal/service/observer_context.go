package service

import "fmt"

// AssembleDynamicContext builds the per-scope markdown dynamic context bundle
// for an observer session. Dispatches to the appropriate scope builder.
func AssembleDynamicContext(s *ObserverService, scope, projectID, workflowID string) (string, error) {
	switch scope {
	case "workflow":
		return buildWorkflowDynamicContext(s, projectID, workflowID)
	case "project":
		return buildProjectDynamicContext(s, projectID)
	case "global":
		return buildGlobalDynamicContext(s)
	default:
		return "", fmt.Errorf("unknown observer scope: %q", scope)
	}
}
