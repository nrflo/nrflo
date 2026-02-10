package api

import (
	"net/http"
	"strings"

	"nrworkflow/internal/db"
	"nrworkflow/internal/service"
	"nrworkflow/internal/types"
	"nrworkflow/internal/ws"
)

// handleListWorkflowDefs returns all workflow definitions for a project
func (s *Server) handleListWorkflowDefs(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project is required")
		return
	}

	database, err := s.getDatabase()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer database.Close()

	pool := db.WrapAsPool(database)
	svc := service.NewWorkflowService(pool)

	defs, err := svc.ListWorkflowDefs(projectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, defs)
}

// handleCreateWorkflowDef creates a new workflow definition
func (s *Server) handleCreateWorkflowDef(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project is required")
		return
	}

	var req types.WorkflowDefCreateRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.ID == "" {
		writeError(w, http.StatusBadRequest, "id is required")
		return
	}
	if len(req.Phases) == 0 {
		writeError(w, http.StatusBadRequest, "phases are required")
		return
	}

	database, err := s.getDatabase()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer database.Close()

	pool := db.WrapAsPool(database)
	svc := service.NewWorkflowService(pool)

	wf, err := svc.CreateWorkflowDef(projectID, &req)
	if err != nil {
		if strings.Contains(err.Error(), "already exists") {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if s.wsHub != nil {
		event := ws.NewEvent(ws.EventWorkflowDefCreated, projectID, "", "", map[string]interface{}{
			"workflow_id": req.ID,
		})
		s.wsHub.Broadcast(event)
	}

	writeJSON(w, http.StatusCreated, wf)
}

// handleGetWorkflowDef returns a single workflow definition
func (s *Server) handleGetWorkflowDef(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project is required")
		return
	}

	id := extractID(r)

	database, err := s.getDatabase()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer database.Close()

	pool := db.WrapAsPool(database)
	svc := service.NewWorkflowService(pool)

	wf, err := svc.GetWorkflowDef(projectID, id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, wf)
}

// handleUpdateWorkflowDef updates a workflow definition
func (s *Server) handleUpdateWorkflowDef(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project is required")
		return
	}

	id := extractID(r)

	var req types.WorkflowDefUpdateRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	database, err := s.getDatabase()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer database.Close()

	pool := db.WrapAsPool(database)
	svc := service.NewWorkflowService(pool)

	if err := svc.UpdateWorkflowDef(projectID, id, &req); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if s.wsHub != nil {
		event := ws.NewEvent(ws.EventWorkflowDefUpdated, projectID, "", "", map[string]interface{}{
			"workflow_id": id,
		})
		s.wsHub.Broadcast(event)
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// handleDeleteWorkflowDef deletes a workflow definition
func (s *Server) handleDeleteWorkflowDef(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project is required")
		return
	}

	id := extractID(r)

	database, err := s.getDatabase()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer database.Close()

	pool := db.WrapAsPool(database)
	svc := service.NewWorkflowService(pool)

	if err := svc.DeleteWorkflowDef(projectID, id); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if s.wsHub != nil {
		event := ws.NewEvent(ws.EventWorkflowDefDeleted, projectID, "", "", map[string]interface{}{
			"workflow_id": id,
		})
		s.wsHub.Broadcast(event)
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
