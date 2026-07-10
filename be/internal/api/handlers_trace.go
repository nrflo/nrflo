package api

import (
	"net/http"
	"strings"

	"be/internal/repo"
	"be/internal/service"
)

// handleGetWorkflowTrace returns the read-time span tree for one workflow
// instance (lanes, layer bands, markers, sub-workflow children).
// GET /api/v1/workflow-instances/{iid}/trace?categories=…&marker_limit=…
func (s *Server) handleGetWorkflowTrace(w http.ResponseWriter, r *http.Request) {
	iid := r.PathValue("iid")
	if iid == "" {
		writeError(w, http.StatusBadRequest, "workflow instance ID required")
		return
	}

	opts, err := service.ParseTraceOptions(r.URL.Query().Get("categories"), r.URL.Query().Get("marker_limit"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	svc := service.NewWorkflowService(s.pool, s.clock)
	trace, err := svc.BuildTrace(iid, opts)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Same guard as findings history: the instance's project must exist.
	if _, err := repo.NewProjectRepo(s.pool, s.clock).Get(trace.ProjectID); err != nil {
		writeError(w, http.StatusForbidden, "access denied")
		return
	}

	writeJSON(w, http.StatusOK, trace)
}
