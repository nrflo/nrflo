package api

import (
	"net/http"

	"be/internal/repo"
	"be/internal/service"
)

// handleGetSessionFlow returns GET /api/v1/sessions/{sid}/flow: the
// read-time transitive closure over delegations/consults/sub-workflow
// children/origin attribution/console siblings rooted at sid. Guard order
// mirrors handleGetWorkflowTrace: build, then verify the root session's
// project still exists.
func (s *Server) handleGetSessionFlow(w http.ResponseWriter, r *http.Request) {
	sid := r.PathValue("sid")
	if sid == "" {
		writeError(w, http.StatusBadRequest, "session id required")
		return
	}

	sess, err := repo.NewAgentSessionRepo(s.pool, s.clock).Get(sid)
	if err != nil {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	if _, err := repo.NewProjectRepo(s.pool, s.clock).Get(sess.ProjectID); err != nil {
		writeError(w, http.StatusForbidden, "access denied")
		return
	}

	flow, err := service.BuildSessionFlow(s.pool, s.clock, sid)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, flow)
}

// handleGetSessionStats returns GET /api/v1/sessions/{sid}/stats: tool-call
// distribution plus cost/token rollup over sid's flow graph. Same guard
// order as handleGetSessionFlow.
func (s *Server) handleGetSessionStats(w http.ResponseWriter, r *http.Request) {
	sid := r.PathValue("sid")
	if sid == "" {
		writeError(w, http.StatusBadRequest, "session id required")
		return
	}

	sess, err := repo.NewAgentSessionRepo(s.pool, s.clock).Get(sid)
	if err != nil {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	if _, err := repo.NewProjectRepo(s.pool, s.clock).Get(sess.ProjectID); err != nil {
		writeError(w, http.StatusForbidden, "access denied")
		return
	}

	flow, err := service.BuildSessionFlow(s.pool, s.clock, sid)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	stats, err := service.BuildSessionStats(s.pool, s.clock, flow)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, stats)
}
