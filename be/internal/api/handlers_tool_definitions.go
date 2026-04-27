package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"be/internal/model"
	"be/internal/repo"
)

// toolDefCreateRequest accepts a JSON body for creating a tool definition.
type toolDefCreateRequest struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
	Endpoint    string          `json:"endpoint"`
	AuthMethod  string          `json:"auth_method,omitempty"`
	AuthRef     *string         `json:"auth_ref,omitempty"`
	TimeoutSec  int             `json:"timeout_sec,omitempty"`
	ProjectID   *string         `json:"project_id,omitempty"`
	WorkflowID  *string         `json:"workflow_id,omitempty"`
}

// toolDefUpdateRequest accepts a JSON body for updating a tool definition.
type toolDefUpdateRequest struct {
	Name        *string          `json:"name,omitempty"`
	Description *string          `json:"description,omitempty"`
	InputSchema *json.RawMessage `json:"input_schema,omitempty"`
	Endpoint    *string          `json:"endpoint,omitempty"`
	AuthMethod  *string          `json:"auth_method,omitempty"`
	AuthRef     *string          `json:"auth_ref,omitempty"`
	TimeoutSec  *int             `json:"timeout_sec,omitempty"`
	ProjectID   *string          `json:"project_id,omitempty"`
	WorkflowID  *string          `json:"workflow_id,omitempty"`
}

// handleListToolDefinitions returns tool definitions, optionally filtered by
// ?project_id= or ?workflow_id=. No X-Project header required.
func (s *Server) handleListToolDefinitions(w http.ResponseWriter, r *http.Request) {
	r0 := repo.NewToolDefinitionRepo(s.pool, s.clock)

	projectID := r.URL.Query().Get("project_id")
	workflowID := r.URL.Query().Get("workflow_id")

	var (
		defs []*model.ToolDefinition
		err  error
	)
	switch {
	case workflowID != "":
		defs, err = r0.ListByWorkflow(workflowID)
	case projectID != "":
		defs, err = r0.ListByProject(projectID)
	default:
		defs, err = r0.List()
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, defs)
}

func (s *Server) handleCreateToolDefinition(w http.ResponseWriter, r *http.Request) {
	var req toolDefCreateRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.ID == "" {
		writeError(w, http.StatusBadRequest, "id is required")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.Endpoint == "" {
		writeError(w, http.StatusBadRequest, "endpoint is required")
		return
	}
	if len(req.InputSchema) == 0 {
		writeError(w, http.StatusBadRequest, "input_schema is required")
		return
	}
	if !json.Valid(req.InputSchema) {
		writeError(w, http.StatusBadRequest, "input_schema must be valid JSON")
		return
	}

	def := &model.ToolDefinition{
		ID:          req.ID,
		Name:        req.Name,
		Description: req.Description,
		InputSchema: req.InputSchema,
		Endpoint:    req.Endpoint,
		AuthMethod:  req.AuthMethod,
		AuthRef:     req.AuthRef,
		TimeoutSec:  req.TimeoutSec,
		ProjectID:   req.ProjectID,
		WorkflowID:  req.WorkflowID,
	}

	r0 := repo.NewToolDefinitionRepo(s.pool, s.clock)
	if err := r0.Create(def); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			writeError(w, http.StatusConflict, "tool definition already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	saved, err := r0.Get(def.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, saved)
}

func (s *Server) handleGetToolDefinition(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	r0 := repo.NewToolDefinitionRepo(s.pool, s.clock)
	def, err := r0.Get(id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, def)
}

func (s *Server) handleUpdateToolDefinition(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req toolDefUpdateRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	fields := &repo.ToolDefUpdateFields{
		Name:        req.Name,
		Description: req.Description,
		Endpoint:    req.Endpoint,
		AuthMethod:  req.AuthMethod,
		AuthRef:     req.AuthRef,
		TimeoutSec:  req.TimeoutSec,
		ProjectID:   req.ProjectID,
		WorkflowID:  req.WorkflowID,
	}
	if req.InputSchema != nil {
		if !json.Valid(*req.InputSchema) {
			writeError(w, http.StatusBadRequest, "input_schema must be valid JSON")
			return
		}
		s := string(*req.InputSchema)
		fields.InputSchema = &s
	}

	r0 := repo.NewToolDefinitionRepo(s.pool, s.clock)
	if err := r0.Update(id, fields); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	def, err := r0.Get(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, def)
}

func (s *Server) handleDeleteToolDefinition(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	r0 := repo.NewToolDefinitionRepo(s.pool, s.clock)
	if err := r0.Delete(id); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
