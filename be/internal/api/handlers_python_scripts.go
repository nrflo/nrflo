package api

import (
	"net/http"
	"strings"

	"be/internal/service"
	"be/internal/types"
)

// handleListPythonScripts returns all python scripts for a project
func (s *Server) handleListPythonScripts(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project is required")
		return
	}

	svc := service.NewPythonScriptService(s.pool, s.clock)
	scripts, err := svc.List(projectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, scripts)
}

// handleCreatePythonScript creates a new python script
func (s *Server) handleCreatePythonScript(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project is required")
		return
	}

	var req types.PythonScriptCreateRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	svc := service.NewPythonScriptService(s.pool, s.clock)
	script, err := svc.Create(projectID, &req)
	if err != nil {
		if strings.Contains(err.Error(), "already exists") {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, script)
}

// handleGetPythonScript returns a single python script
func (s *Server) handleGetPythonScript(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project is required")
		return
	}

	id := r.PathValue("id")

	svc := service.NewPythonScriptService(s.pool, s.clock)
	script, err := svc.Get(projectID, id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, script)
}

// handleUpdatePythonScript updates a python script
func (s *Server) handleUpdatePythonScript(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project is required")
		return
	}

	id := r.PathValue("id")

	var req types.PythonScriptUpdateRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	svc := service.NewPythonScriptService(s.pool, s.clock)
	if err := svc.Update(projectID, id, &req); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		if strings.Contains(err.Error(), "cannot be empty") {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// handleDeletePythonScript deletes a python script
func (s *Server) handleDeletePythonScript(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project is required")
		return
	}

	id := r.PathValue("id")

	svc := service.NewPythonScriptService(s.pool, s.clock)
	if err := svc.Delete(projectID, id); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// handleValidatePythonScript validates Python code syntax without writing to DB
func (s *Server) handleValidatePythonScript(w http.ResponseWriter, r *http.Request) {
	var req types.ValidatePythonScriptRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Code == "" {
		writeError(w, http.StatusBadRequest, "code is required")
		return
	}

	validator := service.NewPythonScriptValidator()
	result := validator.Validate(r.Context(), req.Code)
	writeJSON(w, http.StatusOK, result)
}
