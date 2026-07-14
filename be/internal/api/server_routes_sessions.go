package api

import "net/http"

// registerSessionRoutes registers observer and console session routes.
// Split out of server.go to keep that file's line count under its baseline.
//
// The close route's path parameter is named {sid}, not {id}: requireProjectAdmin
// resolves project scope from {id} first, which would misinterpret the session
// id as a project id. It is registered `protected` instead, with an in-handler
// authorization check (admin user, matching/global service principal, or the
// console session's own bearer) — the last of which lets a console session
// close itself, something requireProjectAdmin could never satisfy since a
// bearer request never populates the user context.
func (s *Server) registerSessionRoutes(protected, projectAdmin func(string, http.HandlerFunc)) {
	// Observer sessions
	protected("POST /api/v1/observers", s.handleLaunchObserver)
	protected("GET /api/v1/observers", s.handleListObservers)

	// Console sessions
	projectAdmin("POST /api/v1/console/sessions", s.handleCreateConsoleSession)
	protected("POST /api/v1/console/sessions/{sid}/close", s.handleCloseConsoleSession)
}
