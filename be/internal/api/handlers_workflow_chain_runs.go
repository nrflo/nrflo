package api

import (
	"net/http"
	"strings"

	"be/internal/service"
	"be/internal/ws"
)

type startChainRunRequest struct {
	Instructions string `json:"instructions"`
	TriggeredBy  string `json:"triggered_by"`
}

func (s *Server) handleStartChainRun(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "X-Project header required")
		return
	}
	chainID := r.PathValue("id")

	var req startChainRunRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.TriggeredBy == "" {
		if u := getUser(r); u != nil {
			req.TriggeredBy = u.Email
		}
	}

	svc := service.NewWorkflowChainRunService(s.pool, s.clock)
	detail, err := svc.CreateRun(projectID, chainID, req.Instructions, req.TriggeredBy)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if s.wfChainRunner != nil {
		if err := s.wfChainRunner.Start(r.Context(), detail.ID); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	s.wsHub.Broadcast(ws.NewEvent(ws.EventChainRunStarted, projectID, "", "", map[string]interface{}{
		"run_id":   detail.ID,
		"chain_id": chainID,
	}))
	writeJSON(w, http.StatusCreated, detail)
}

func (s *Server) handleCancelChainRun(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "X-Project header required")
		return
	}
	runID := r.PathValue("runId")

	svc := service.NewWorkflowChainRunService(s.pool, s.clock)
	detail, err := svc.GetRunDetail(projectID, runID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if detail.Status == "completed" || detail.Status == "failed" || detail.Status == "canceled" {
		writeError(w, http.StatusBadRequest, "run is already in terminal state")
		return
	}

	if s.wfChainRunner != nil {
		if err := s.wfChainRunner.Cancel(runID); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListChainRuns(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "X-Project header required")
		return
	}
	chainID := r.PathValue("id")

	svc := service.NewWorkflowChainRunService(s.pool, s.clock)
	runs, err := svc.ListRuns(projectID, chainID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, runs)
}

func (s *Server) handleGetChainRun(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "X-Project header required")
		return
	}
	runID := r.PathValue("runId")

	svc := service.NewWorkflowChainRunService(s.pool, s.clock)
	detail, err := svc.GetRunDetail(projectID, runID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, detail)
}
