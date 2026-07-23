package api

import "net/http"

// registerInstanceReadRoutes registers the workflow-instance-scoped read
// routes (artifacts, trace, stepwise cursor progress). Split out of
// server.go to keep that file's line count under its baseline (mirrors
// registerObservabilityRoutes).
func (s *Server) registerInstanceReadRoutes(protected func(string, http.HandlerFunc)) {
	protected("GET /api/v1/workflow-instances/{iid}/artifacts", s.handleListArtifacts)
	protected("GET /api/v1/workflow-instances/{iid}/trace", s.handleGetWorkflowTrace)
	protected("GET /api/v1/workflow-instances/{iid}/steps", s.handleGetStepCursors)
}
