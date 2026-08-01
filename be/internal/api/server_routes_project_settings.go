package api

import "net/http"

// registerProjectSettingsRoutes registers the per-project settings routes:
// artifact storage, workflow cleanup, observer, capture-thinking, and external
// MCP servers. Split out of server.go to keep that file's line count under its
// baseline (mirrors registerObservabilityRoutes).
func (s *Server) registerProjectSettingsRoutes(protected, projectAdmin func(string, http.HandlerFunc)) {
	protected("GET /api/v1/projects/{id}/settings/artifact-storage", s.handleGetProjectArtifactStorage)
	projectAdmin("PUT /api/v1/projects/{id}/settings/artifact-storage", s.handlePutProjectArtifactStorage)
	protected("GET /api/v1/projects/{id}/settings/cleanup", s.handleGetProjectCleanup)
	projectAdmin("PUT /api/v1/projects/{id}/settings/cleanup", s.handlePutProjectCleanup)
	protected("GET /api/v1/projects/{id}/settings/observer", s.handleGetProjectObserver)
	projectAdmin("PUT /api/v1/projects/{id}/settings/observer", s.handlePutProjectObserver)
	protected("GET /api/v1/projects/{id}/settings/capture-thinking", s.handleGetProjectCaptureThinking)
	projectAdmin("PUT /api/v1/projects/{id}/settings/capture-thinking", s.handlePutProjectCaptureThinking)
	protected("GET /api/v1/projects/{id}/settings/mcp-servers", s.handleGetProjectMCPServers)
	projectAdmin("PUT /api/v1/projects/{id}/settings/mcp-servers", s.handlePutProjectMCPServers)
}
