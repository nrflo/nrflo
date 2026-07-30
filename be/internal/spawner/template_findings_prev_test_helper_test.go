package spawner

// fetchPreviousDataAndReason is a test-only convenience wrapper around the
// production resolvePrevContinuedSession/previousDataFor split (see
// template_findings_prev.go) so existing tests can keep calling the old
// combined signature.
func (s *Spawner) fetchPreviousDataAndReason(projectID, ticketID, workflowName, agentType, modelID, phase, instanceID string) (data string, resultReason string) {
	prev, wfiID, ok := s.resolvePrevContinuedSession(projectID, ticketID, workflowName, agentType, modelID, phase, instanceID)
	if !ok {
		return "", ""
	}
	return s.previousDataFor(prev, wfiID, agentType, projectID, workflowName, phase), prev.reason
}
