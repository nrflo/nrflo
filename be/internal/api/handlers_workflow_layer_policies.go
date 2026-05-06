package api

import (
	"net/http"
	"strconv"

	"be/internal/service"
	"be/internal/ws"
)

// handleListLayerPolicies returns all per-layer pass policies for a workflow.
func (s *Server) handleListLayerPolicies(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project is required")
		return
	}

	workflowID := r.PathValue("wid")

	svc := service.NewWorkflowLayerPolicyService(s.pool, s.clock)
	policies, err := svc.GetLayerPolicies(projectID, workflowID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, policies)
}

// handleSetLayerPolicy upserts the pass_policy for a specific layer.
func (s *Server) handleSetLayerPolicy(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project is required")
		return
	}

	workflowID := r.PathValue("wid")
	layerStr := r.PathValue("layer")
	layer, err := strconv.Atoi(layerStr)
	if err != nil || layer < 0 {
		writeError(w, http.StatusBadRequest, "layer must be a non-negative integer")
		return
	}

	var req struct {
		PassPolicy string `json:"pass_policy"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.PassPolicy == "" {
		writeError(w, http.StatusBadRequest, "pass_policy is required")
		return
	}

	svc := service.NewWorkflowLayerPolicyService(s.pool, s.clock)
	if err := svc.SetLayerPolicy(projectID, workflowID, layer, req.PassPolicy); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if s.wsHub != nil {
		s.wsHub.Broadcast(ws.NewEvent(ws.EventWorkflowDefUpdated, projectID, "", "", map[string]interface{}{
			"workflow_id": workflowID,
		}))
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleDeleteLayerPolicy removes the pass_policy for a specific layer (resets to default "any").
func (s *Server) handleDeleteLayerPolicy(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project is required")
		return
	}

	workflowID := r.PathValue("wid")
	layerStr := r.PathValue("layer")
	layer, err := strconv.Atoi(layerStr)
	if err != nil || layer < 0 {
		writeError(w, http.StatusBadRequest, "layer must be a non-negative integer")
		return
	}

	svc := service.NewWorkflowLayerPolicyService(s.pool, s.clock)
	if err := svc.DeleteLayerPolicy(projectID, workflowID, layer); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if s.wsHub != nil {
		s.wsHub.Broadcast(ws.NewEvent(ws.EventWorkflowDefUpdated, projectID, "", "", map[string]interface{}{
			"workflow_id": workflowID,
		}))
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
