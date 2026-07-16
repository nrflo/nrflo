package api

import (
	"errors"
	"net/http"
	"strings"

	"be/internal/service"
)

// writeServiceError maps a service-layer error to an HTTP status and writes it.
// It centralizes the create/update mapping shared by the agent-definition and
// system-agent-definition handlers:
//   - ErrAPIModeDisabled          -> 400 (code "api_mode_disabled")
//   - "already exists"            -> 409
//   - "not found"                 -> 404
//   - ErrValidation (user input)  -> 400
//   - anything else (internal/DB) -> 500
func writeServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, service.ErrAPIModeDisabled):
		writeError(w, http.StatusBadRequest, "api_mode_disabled")
	case strings.Contains(err.Error(), "already exists"):
		writeError(w, http.StatusConflict, err.Error())
	case strings.Contains(err.Error(), "not found"):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, service.ErrValidation):
		writeError(w, http.StatusBadRequest, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, err.Error())
	}
}
