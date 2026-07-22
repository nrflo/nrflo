package api

import "net/http"

// registerCustomProviderRoutes registers the custom_providers registry CRUD
// routes. Split out of server.go to keep that file's line count under its
// baseline (mirrors registerSessionRoutes).
func (s *Server) registerCustomProviderRoutes(admin func(string, http.HandlerFunc)) {
	admin("GET /api/v1/custom-providers", s.handleListCustomProviders)
	admin("POST /api/v1/custom-providers", s.handleCreateCustomProvider)
	admin("GET /api/v1/custom-providers/{name}", s.handleGetCustomProvider)
	admin("PATCH /api/v1/custom-providers/{name}", s.handleUpdateCustomProvider)
	admin("DELETE /api/v1/custom-providers/{name}", s.handleDeleteCustomProvider)
	admin("POST /api/v1/custom-providers/check", s.handleCheckCustomProvider)
}
