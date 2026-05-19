package socket

import (
	"context"
	"encoding/json"
	"strings"

	"be/internal/model"
	"be/internal/repo"
	"be/internal/types"
)

func (h *Handler) handleObserverWorkflow(ctx context.Context, req Request, action string, session *model.AgentSession, base observerBaseParams) Response {
	switch action {
	case "show":
		return h.obsWorkflowShow(req, session, base)
	case "runs":
		return h.obsWorkflowRuns(req, session, base)
	case "findings":
		return h.obsWorkflowFindings(req, session)
	case "logs":
		return h.obsWorkflowLogs(req, session)
	case "trigger":
		return h.obsWorkflowTrigger(ctx, req, session, base)
	case "retry_failed":
		return h.obsWorkflowRetryFailed(ctx, req, session)
	case "def.update":
		return h.obsWorkflowDefUpdate(req, session, base)
	default:
		return MakeErrorResponse(req.ID, NewMethodNotFoundError("observer.workflow."+action))
	}
}

func (h *Handler) obsWorkflowShow(req Request, session *model.AgentSession, base observerBaseParams) Response {
	projectID, workflowID, errR := h.resolveWorkflowScope(session, base)
	if errR != nil {
		return MakeErrorResponse(req.ID, errR)
	}
	def, err := h.workflowSvc.GetWorkflowDef(projectID, workflowID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return MakeErrorResponse(req.ID, NewNotFoundError(err.Error()))
		}
		return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
	}
	return MakeResponse(req.ID, def)
}

func (h *Handler) obsWorkflowRuns(req Request, session *model.AgentSession, base observerBaseParams) Response {
	projectID, workflowID, errR := h.resolveWorkflowScope(session, base)
	if errR != nil {
		return MakeErrorResponse(req.ID, errR)
	}
	wfi, err := repo.NewWorkflowInstanceRepo(h.pool, h.clk).Get(session.WorkflowInstanceID)
	if err != nil {
		return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
	}
	instances, err := h.workflowSvc.ListWorkflowInstances(projectID, wfi.TicketID)
	if err != nil {
		return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
	}
	var filtered []*model.WorkflowInstance
	for _, inst := range instances {
		if inst.WorkflowID == workflowID {
			filtered = append(filtered, inst)
		}
	}
	return MakeResponse(req.ID, filtered)
}

func (h *Handler) obsWorkflowFindings(req Request, session *model.AgentSession) Response {
	wfi, err := repo.NewWorkflowInstanceRepo(h.pool, h.clk).Get(session.WorkflowInstanceID)
	if err != nil {
		return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
	}
	var params types.FindingsGetRequest
	if req.Params != nil {
		json.Unmarshal(req.Params, &params) //nolint:errcheck
	}
	if params.InstanceID == "" {
		params.InstanceID = wfi.ID
	}
	findings, err := h.findingsSvc.Get(&params)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return MakeErrorResponse(req.ID, NewNotFoundError(err.Error()))
		}
		return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
	}
	return MakeResponse(req.ID, findings)
}

func (h *Handler) obsWorkflowLogs(req Request, session *model.AgentSession) Response {
	var params struct {
		TargetSessionID string `json:"target_session_id"`
		Limit           int    `json:"limit"`
		Offset          int    `json:"offset"`
	}
	if req.Params != nil {
		json.Unmarshal(req.Params, &params) //nolint:errcheck
	}
	wfi, err := repo.NewWorkflowInstanceRepo(h.pool, h.clk).Get(session.WorkflowInstanceID)
	if err != nil {
		return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
	}
	sessions, err := h.agentSvc.GetTicketSessions(session.ProjectID, wfi.TicketID, wfi.WorkflowID)
	if err != nil {
		return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
	}
	targetID := params.TargetSessionID
	if targetID == "" && len(sessions) > 0 {
		targetID = sessions[0].ID
	}
	if targetID == "" {
		return MakeResponse(req.ID, map[string]interface{}{"messages": []interface{}{}, "total": 0})
	}
	msgs, total, err := h.agentSvc.GetSessionMessages(targetID, params.Limit, params.Offset, "")
	if err != nil {
		return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
	}
	return MakeResponse(req.ID, map[string]interface{}{"messages": msgs, "total": total})
}

func (h *Handler) obsWorkflowTrigger(ctx context.Context, req Request, session *model.AgentSession, base observerBaseParams) Response {
	var params struct {
		TicketID     string `json:"ticket_id"`
		Instructions string `json:"instructions"`
		ScopeType    string `json:"scope_type"`
	}
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return MakeErrorResponse(req.ID, NewInvalidParamsError(err.Error()))
		}
	}
	if h.workflowRunner == nil {
		return MakeErrorResponse(req.ID, NewInternalError("workflow runner not available"))
	}
	projectID, workflowID, errR := h.resolveWorkflowScope(session, base)
	if errR != nil {
		return MakeErrorResponse(req.ID, errR)
	}
	scopeType := params.ScopeType
	if scopeType == "" {
		scopeType = "ticket"
	}
	instanceID, err := h.workflowRunner.StartWorkflow(ctx, projectID, params.TicketID, workflowID, params.Instructions, scopeType)
	if err != nil {
		return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
	}
	return MakeResponse(req.ID, map[string]string{"instance_id": instanceID, "status": "started"})
}

func (h *Handler) obsWorkflowRetryFailed(ctx context.Context, req Request, session *model.AgentSession) Response {
	var params struct {
		TargetSessionID string `json:"target_session_id"`
	}
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return MakeErrorResponse(req.ID, NewInvalidParamsError(err.Error()))
		}
	}
	if h.workflowRunner == nil {
		return MakeErrorResponse(req.ID, NewInternalError("workflow runner not available"))
	}
	wfi, err := repo.NewWorkflowInstanceRepo(h.pool, h.clk).Get(session.WorkflowInstanceID)
	if err != nil {
		return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
	}
	if wfi.TicketID != "" {
		if err := h.workflowRunner.RetryFailed(ctx, session.ProjectID, wfi.TicketID, wfi.WorkflowID, params.TargetSessionID); err != nil {
			return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
		}
	} else {
		if err := h.workflowRunner.RetryFailedProject(ctx, session.ProjectID, wfi.WorkflowID, params.TargetSessionID, wfi.ID); err != nil {
			return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
		}
	}
	return MakeResponse(req.ID, map[string]string{"status": "retrying"})
}

func (h *Handler) obsWorkflowDefUpdate(req Request, session *model.AgentSession, base observerBaseParams) Response {
	projectID, workflowID, errR := h.resolveWorkflowScope(session, base)
	if errR != nil {
		return MakeErrorResponse(req.ID, errR)
	}
	var params types.WorkflowDefUpdateRequest
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return MakeErrorResponse(req.ID, NewInvalidParamsError(err.Error()))
	}
	if err := h.workflowSvc.UpdateWorkflowDef(projectID, workflowID, &params); err != nil {
		if strings.Contains(err.Error(), "not found") {
			return MakeErrorResponse(req.ID, NewNotFoundError(err.Error()))
		}
		return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
	}
	return MakeResponse(req.ID, map[string]string{"status": "updated"})
}

// resolveWorkflowScope returns projectID and workflowID, defaulting from the session's
// workflow instance when base params are empty.
func (h *Handler) resolveWorkflowScope(session *model.AgentSession, base observerBaseParams) (projectID, workflowID string, errR *ErrorInfo) {
	projectID = base.ProjectID
	if projectID == "" {
		projectID = session.ProjectID
	}
	workflowID = base.WorkflowID
	if workflowID == "" {
		wfi, err := repo.NewWorkflowInstanceRepo(h.pool, h.clk).Get(session.WorkflowInstanceID)
		if err != nil {
			return "", "", NewInternalError("failed to load workflow instance")
		}
		workflowID = wfi.WorkflowID
	}
	return projectID, workflowID, nil
}
