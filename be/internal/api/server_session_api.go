package api

import (
	"net/http"
	"strings"
)

// withSessionForAPI applies SCS LoadAndSave only for /api/ path prefix.
// Static UI routes are excluded so session cookies are not set on SPA page loads.
func (s *Server) withSessionForAPI(next http.Handler) http.Handler {
	if s.sessionMgr == nil {
		return next
	}
	ls := s.sessionMgr.LoadAndSave(next)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			ls.ServeHTTP(w, r)
		} else {
			next.ServeHTTP(w, r)
		}
	})
}
