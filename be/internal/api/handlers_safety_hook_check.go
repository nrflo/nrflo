package api

import (
	"net/http"

	"be/internal/spawner"
)

type safetyHookCheckRequest struct {
	Config  spawner.SafetyHookConfig `json:"config"`
	Command string                   `json:"command"`
}

type safetyHookCheckResponse struct {
	Allowed bool   `json:"allowed"`
	Reason  string `json:"reason"`
}

func (s *Server) handleCheckSafetyHook(w http.ResponseWriter, r *http.Request) {
	var req safetyHookCheckRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Command == "" {
		writeError(w, http.StatusBadRequest, "command is required")
		return
	}

	allowed, reason, err := spawner.CheckSafetyHook(req.Config, req.Command)
	if err != nil {
		writeJSON(w, http.StatusOK, &safetyHookCheckResponse{
			Allowed: false,
			Reason:  err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, &safetyHookCheckResponse{
		Allowed: allowed,
		Reason:  reason,
	})
}
