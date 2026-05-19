package socket

import (
	"context"
	"encoding/json"
	"strings"

	"be/internal/model"
	"be/internal/types"
)

func (h *Handler) handleObserverProject(_ context.Context, req Request, action string, session *model.AgentSession, base observerBaseParams) Response {
	switch action {
	case "workflows":
		return h.obsProjectWorkflows(req, session, base)
	case "runs":
		return h.obsProjectRuns(req, session, base)
	case "findings":
		return h.obsProjectFindings(req, session, base)
	case "env.list":
		return h.obsProjectEnvList(req, session, base)
	case "env.set":
		return h.obsProjectEnvSet(req, session, base)
	case "env.unset":
		return h.obsProjectEnvUnset(req, session, base)
	case "workflow.create":
		return h.obsProjectWorkflowCreate(req, session, base)
	case "workflow.delete":
		return h.obsProjectWorkflowDelete(req, session, base)
	default:
		return MakeErrorResponse(req.ID, NewMethodNotFoundError("observer.project."+action))
	}
}

func (h *Handler) obsProjectWorkflows(req Request, session *model.AgentSession, base observerBaseParams) Response {
	projectID := base.ProjectID
	if projectID == "" {
		projectID = session.ProjectID
	}
	defs, err := h.workflowSvc.ListWorkflowDefs(projectID)
	if err != nil {
		return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
	}
	return MakeResponse(req.ID, defs)
}

func (h *Handler) obsProjectRuns(req Request, session *model.AgentSession, base observerBaseParams) Response {
	projectID := base.ProjectID
	if projectID == "" {
		projectID = session.ProjectID
	}
	instances, err := h.workflowSvc.ListProjectWorkflowInstances(projectID)
	if err != nil {
		return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
	}
	return MakeResponse(req.ID, instances)
}

func (h *Handler) obsProjectFindings(req Request, session *model.AgentSession, base observerBaseParams) Response {
	projectID := base.ProjectID
	if projectID == "" {
		projectID = session.ProjectID
	}
	var params types.ProjectFindingsGetRequest
	if req.Params != nil {
		json.Unmarshal(req.Params, &params) //nolint:errcheck
	}
	findings, err := h.projectFindingsSvc.Get(projectID, &params)
	if err != nil {
		return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
	}
	return MakeResponse(req.ID, findings)
}

func (h *Handler) obsProjectEnvList(req Request, session *model.AgentSession, base observerBaseParams) Response {
	projectID := base.ProjectID
	if projectID == "" {
		projectID = session.ProjectID
	}
	vars, err := h.projectEnvVarSvc.List(projectID)
	if err != nil {
		return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
	}
	return MakeResponse(req.ID, vars)
}

func (h *Handler) obsProjectEnvSet(req Request, session *model.AgentSession, base observerBaseParams) Response {
	var params struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return MakeErrorResponse(req.ID, NewInvalidParamsError(err.Error()))
	}
	if params.Name == "" {
		return MakeErrorResponse(req.ID, NewValidationError("name is required"))
	}
	projectID := base.ProjectID
	if projectID == "" {
		projectID = session.ProjectID
	}
	v, err := h.projectEnvVarSvc.Upsert(projectID, params.Name, params.Value)
	if err != nil {
		if strings.Contains(err.Error(), "invalid") || strings.Contains(err.Error(), "reserved") {
			return MakeErrorResponse(req.ID, NewValidationError(err.Error()))
		}
		return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
	}
	return MakeResponse(req.ID, v)
}

func (h *Handler) obsProjectEnvUnset(req Request, session *model.AgentSession, base observerBaseParams) Response {
	var params struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return MakeErrorResponse(req.ID, NewInvalidParamsError(err.Error()))
	}
	if params.Name == "" {
		return MakeErrorResponse(req.ID, NewValidationError("name is required"))
	}
	projectID := base.ProjectID
	if projectID == "" {
		projectID = session.ProjectID
	}
	if err := h.projectEnvVarSvc.Delete(projectID, params.Name); err != nil {
		return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
	}
	return MakeResponse(req.ID, map[string]string{"status": "deleted"})
}

func (h *Handler) obsProjectWorkflowCreate(req Request, session *model.AgentSession, base observerBaseParams) Response {
	projectID := base.ProjectID
	if projectID == "" {
		projectID = session.ProjectID
	}
	var params types.WorkflowDefCreateRequest
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return MakeErrorResponse(req.ID, NewInvalidParamsError(err.Error()))
	}
	wf, err := h.workflowSvc.CreateWorkflowDef(projectID, &params)
	if err != nil {
		if strings.Contains(err.Error(), "already exists") {
			return MakeErrorResponse(req.ID, NewConflictError(err.Error()))
		}
		return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
	}
	return MakeResponse(req.ID, wf)
}

func (h *Handler) obsProjectWorkflowDelete(req Request, session *model.AgentSession, base observerBaseParams) Response {
	projectID := base.ProjectID
	if projectID == "" {
		projectID = session.ProjectID
	}
	workflowID := base.WorkflowID
	if workflowID == "" {
		return MakeErrorResponse(req.ID, NewValidationError("workflow_id is required"))
	}
	if err := h.workflowSvc.DeleteWorkflowDef(projectID, workflowID); err != nil {
		if strings.Contains(err.Error(), "not found") {
			return MakeErrorResponse(req.ID, NewNotFoundError(err.Error()))
		}
		return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
	}
	return MakeResponse(req.ID, map[string]string{"status": "deleted"})
}
