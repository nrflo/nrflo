package api

import (
	"net/http"
	"strings"

	"be/internal/service"
)

// handleListProviders returns the allowed CLI modes for all providers.
func (s *Server) handleListProviders(w http.ResponseWriter, r *http.Request) {
	svc := service.NewProviderSettingsService(service.NewGlobalSettingsService(s.pool, s.clock))

	all, err := svc.GetAll()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	resp := make(map[string]map[string]interface{}, len(all))
	for provider, modes := range all {
		resp[provider] = map[string]interface{}{"modes": modes}
	}
	writeJSON(w, http.StatusOK, resp)
}

// handlePatchProvider updates the allowed CLI modes for a provider.
func (s *Server) handlePatchProvider(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	var body struct {
		Modes []string `json:"modes"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	svc := service.NewProviderSettingsService(service.NewGlobalSettingsService(s.pool, s.clock))

	if err := svc.SetModes(name, body.Modes); err != nil {
		if strings.Contains(err.Error(), "invalid provider") ||
			strings.Contains(err.Error(), "must not be empty") ||
			strings.Contains(err.Error(), "invalid mode") {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}
