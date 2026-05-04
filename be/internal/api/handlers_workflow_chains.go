package api

import (
	"net/http"
	"strings"

	"be/internal/service"
	"be/internal/types"
	"be/internal/ws"
)

func (s *Server) handleListWorkflowChains(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "X-Project header required")
		return
	}
	svc := service.NewWorkflowChainService(s.pool, s.clock, service.NewWorkflowService(s.pool, s.clock))
	chains, err := svc.ListChains(projectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, chains)
}

func (s *Server) handleCreateWorkflowChain(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "X-Project header required")
		return
	}
	var req types.WorkflowChainCreateRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	svc := service.NewWorkflowChainService(s.pool, s.clock, service.NewWorkflowService(s.pool, s.clock))
	chain, err := svc.CreateChain(projectID, &req)
	if err != nil {
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "already exists") {
			status = http.StatusConflict
		}
		writeError(w, status, err.Error())
		return
	}
	s.wsHub.Broadcast(ws.NewEvent(ws.EventWorkflowChainCreated, projectID, "", "", map[string]interface{}{"chain_id": chain.ID}))
	writeJSON(w, http.StatusCreated, chain)
}

func (s *Server) handleGetWorkflowChain(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "X-Project header required")
		return
	}
	id := r.PathValue("id")
	svc := service.NewWorkflowChainService(s.pool, s.clock, service.NewWorkflowService(s.pool, s.clock))
	chain, err := svc.GetChain(projectID, id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, chain)
}

func (s *Server) handleUpdateWorkflowChain(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "X-Project header required")
		return
	}
	id := r.PathValue("id")
	var req types.WorkflowChainUpdateRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	svc := service.NewWorkflowChainService(s.pool, s.clock, service.NewWorkflowService(s.pool, s.clock))
	chain, err := svc.UpdateChain(projectID, id, &req)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.wsHub.Broadcast(ws.NewEvent(ws.EventWorkflowChainUpdated, projectID, "", "", map[string]interface{}{"chain_id": id}))
	writeJSON(w, http.StatusOK, chain)
}

func (s *Server) handleDeleteWorkflowChain(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "X-Project header required")
		return
	}
	id := r.PathValue("id")
	svc := service.NewWorkflowChainService(s.pool, s.clock, service.NewWorkflowService(s.pool, s.clock))
	if err := svc.DeleteChain(projectID, id); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.wsHub.Broadcast(ws.NewEvent(ws.EventWorkflowChainDeleted, projectID, "", "", map[string]interface{}{"chain_id": id}))
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAppendChainStep(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "X-Project header required")
		return
	}
	id := r.PathValue("id")
	var req types.WorkflowChainStepRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	svc := service.NewWorkflowChainService(s.pool, s.clock, service.NewWorkflowService(s.pool, s.clock))
	chain, err := svc.AppendStep(projectID, id, &req)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.wsHub.Broadcast(ws.NewEvent(ws.EventWorkflowChainUpdated, projectID, "", "", map[string]interface{}{"chain_id": id}))
	writeJSON(w, http.StatusCreated, chain)
}

func (s *Server) handleUpdateChainStep(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "X-Project header required")
		return
	}
	id := r.PathValue("id")
	stepID := r.PathValue("stepId")
	var req types.WorkflowChainStepUpdateRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	svc := service.NewWorkflowChainService(s.pool, s.clock, service.NewWorkflowService(s.pool, s.clock))
	chain, err := svc.UpdateStep(projectID, id, stepID, &req)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.wsHub.Broadcast(ws.NewEvent(ws.EventWorkflowChainUpdated, projectID, "", "", map[string]interface{}{"chain_id": id}))
	writeJSON(w, http.StatusOK, chain)
}

func (s *Server) handleDeleteChainStep(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "X-Project header required")
		return
	}
	id := r.PathValue("id")
	stepID := r.PathValue("stepId")
	svc := service.NewWorkflowChainService(s.pool, s.clock, service.NewWorkflowService(s.pool, s.clock))
	chain, err := svc.DeleteStep(projectID, id, stepID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.wsHub.Broadcast(ws.NewEvent(ws.EventWorkflowChainUpdated, projectID, "", "", map[string]interface{}{"chain_id": id}))
	writeJSON(w, http.StatusOK, chain)
}

func (s *Server) handleReorderChainSteps(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "X-Project header required")
		return
	}
	id := r.PathValue("id")
	var req types.ReorderStepsRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	svc := service.NewWorkflowChainService(s.pool, s.clock, service.NewWorkflowService(s.pool, s.clock))
	chain, err := svc.ReorderSteps(projectID, id, &req)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.wsHub.Broadcast(ws.NewEvent(ws.EventWorkflowChainUpdated, projectID, "", "", map[string]interface{}{"chain_id": id}))
	writeJSON(w, http.StatusOK, chain)
}
