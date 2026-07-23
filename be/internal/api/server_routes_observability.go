package api

import "net/http"

// registerObservabilityRoutes registers the tiering report/apply routes and
// the system-agent-runs listing. Split out of server.go to keep that file's
// line count under its baseline (mirrors registerCustomProviderRoutes).
func (s *Server) registerObservabilityRoutes(admin func(string, http.HandlerFunc)) {
	admin("GET /api/v1/admin/tiering-report", s.handleTieringReport)
	admin("POST /api/v1/admin/tiering-apply", s.handleApplyTiering)
	admin("GET /api/v1/system-agent-runs", s.handleListSystemAgentRuns)
}
