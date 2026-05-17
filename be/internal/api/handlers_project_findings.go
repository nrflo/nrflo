package api

import (
	"encoding/json"
	"net/http"
	"net/url"

	"be/internal/repo"
	"be/internal/service"
	"be/internal/types"
	"be/internal/ws"
)

// requestActor returns an Actor for the current HTTP request principal.
func requestActor(r *http.Request) repo.Actor {
	if sp := getServicePrincipal(r); sp != nil {
		return repo.Actor{ID: sp.TokenID, Source: "service_token"}
	}
	return repo.Actor{ID: getUserID(r), Source: "user"}
}

// handleGetProjectFindings returns all project findings as a JSON map.
// GET /api/v1/projects/{id}/findings
func (s *Server) handleGetProjectFindings(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project ID is required")
		return
	}

	svc := service.NewProjectFindingsService(s.pool, s.clock)
	result, err := svc.Get(projectID, &types.ProjectFindingsGetRequest{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// handleUpsertProjectFinding creates or updates a single project finding.
// POST /api/v1/projects/{id}/findings
func (s *Server) handleUpsertProjectFinding(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project ID is required")
		return
	}

	var req types.ProjectFindingsAddRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if req.Key == "" {
		writeError(w, http.StatusBadRequest, "key is required")
		return
	}

	svc := service.NewProjectFindingsService(s.pool, s.clock)
	if err := svc.Add(projectID, &req, requestActor(r)); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	service.BroadcastFromCtx(s.wsHub, ws.EventProjectFindingsUpdated, service.BroadcastCtx{ProjectID: projectID}, map[string]interface{}{
		"key":    req.Key,
		"action": "add",
	})

	writeJSON(w, http.StatusOK, map[string]string{"status": "saved", "key": req.Key})
}

// handleDeleteProjectFinding deletes a single project finding by key.
// DELETE /api/v1/projects/{id}/findings/{key}
func (s *Server) handleDeleteProjectFinding(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project ID is required")
		return
	}

	rawKey := r.PathValue("key")
	key, err := url.PathUnescape(rawKey)
	if err != nil {
		key = rawKey
	}
	if key == "" {
		writeError(w, http.StatusBadRequest, "key is required")
		return
	}

	svc := service.NewProjectFindingsService(s.pool, s.clock)
	deleted, err := svc.Delete(projectID, &types.ProjectFindingsDeleteRequest{Keys: []string{key}}, requestActor(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if len(deleted) == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "finding not found", "key": key})
		return
	}

	service.BroadcastFromCtx(s.wsHub, ws.EventProjectFindingsUpdated, service.BroadcastCtx{ProjectID: projectID}, map[string]interface{}{
		"action":  "delete",
		"deleted": deleted,
	})

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted", "key": key})
}
