package socket

import (
	"context"
	"encoding/json"
	"strings"

	"be/internal/model"
	"be/internal/types"
)

func (h *Handler) handleObserverGlobal(_ context.Context, req Request, action string, session *model.AgentSession, base observerBaseParams) Response {
	switch action {
	case "projects":
		return h.obsGlobalProjects(req)
	case "recent_sessions":
		return h.obsGlobalRecentSessions(req, session, base)
	case "health":
		return h.obsGlobalHealth(req)
	case "project.create":
		return h.obsGlobalProjectCreate(req)
	case "project.delete":
		return h.obsGlobalProjectDelete(req, base)
	default:
		return MakeErrorResponse(req.ID, NewMethodNotFoundError("observer.global."+action))
	}
}

func (h *Handler) obsGlobalProjects(req Request) Response {
	projects, err := h.projectSvc.List()
	if err != nil {
		return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
	}
	return MakeResponse(req.ID, projects)
}

func (h *Handler) obsGlobalRecentSessions(req Request, session *model.AgentSession, base observerBaseParams) Response {
	var params struct {
		ProjectID string `json:"project_id"`
		Limit     int    `json:"limit"`
	}
	if req.Params != nil {
		json.Unmarshal(req.Params, &params) //nolint:errcheck
	}
	projectID := params.ProjectID
	if projectID == "" {
		projectID = base.ProjectID
	}
	if projectID == "" {
		projectID = session.ProjectID
	}
	sessions, err := h.agentSvc.GetRecentSessions(projectID, params.Limit)
	if err != nil {
		return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
	}
	return MakeResponse(req.ID, sessions)
}

func (h *Handler) obsGlobalHealth(req Request) Response {
	enabled, err := h.globalSettingsSvc.GetExperimentalObserverEnabled()
	if err != nil {
		return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
	}
	var ping int
	dbOK := h.pool.QueryRow("SELECT 1").Scan(&ping) == nil
	return MakeResponse(req.ID, map[string]interface{}{
		"status":           "ok",
		"db":               dbOK,
		"observer_enabled": enabled,
	})
}

func (h *Handler) obsGlobalProjectCreate(req Request) Response {
	var params struct {
		ProjectID     string `json:"project_id"`
		Name          string `json:"name"`
		RootPath      string `json:"root_path"`
		DefaultBranch string `json:"default_branch"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return MakeErrorResponse(req.ID, NewInvalidParamsError(err.Error()))
	}
	if params.ProjectID == "" {
		return MakeErrorResponse(req.ID, NewValidationError("project_id is required"))
	}
	createReq := &types.ProjectCreateRequest{
		Name:          params.Name,
		RootPath:      params.RootPath,
		DefaultBranch: params.DefaultBranch,
	}
	project, err := h.projectSvc.Create(params.ProjectID, createReq)
	if err != nil {
		if strings.Contains(err.Error(), "already exists") {
			return MakeErrorResponse(req.ID, NewConflictError(err.Error()))
		}
		return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
	}
	return MakeResponse(req.ID, project)
}

func (h *Handler) obsGlobalProjectDelete(req Request, base observerBaseParams) Response {
	projectID := base.ProjectID
	if projectID == "" {
		return MakeErrorResponse(req.ID, NewValidationError("project_id is required"))
	}
	if err := h.projectSvc.Delete(projectID); err != nil {
		if strings.Contains(err.Error(), "not found") {
			return MakeErrorResponse(req.ID, NewNotFoundError(err.Error()))
		}
		return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
	}
	return MakeResponse(req.ID, map[string]string{"status": "deleted"})
}
