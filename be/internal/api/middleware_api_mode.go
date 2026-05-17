package api

import (
	"net/http"

	"be/internal/service"
)

// apiModeOnly returns 400 {"error":"api_mode_disabled"} when the api_mode_enabled
// global setting is not "true"; otherwise delegates to next.
func (s *Server) apiModeOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		svc := service.NewGlobalSettingsService(s.pool, s.clock)
		val, _ := svc.Get("api_mode_enabled")
		if val != "true" {
			writeError(w, http.StatusBadRequest, "api_mode_disabled")
			return
		}
		next.ServeHTTP(w, r)
	})
}
