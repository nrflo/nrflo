package socket

import (
	"encoding/json"
	"strings"

	"nrworkflow/internal/types"
)

// Handle dispatches a request to the appropriate service method
func (h *Handler) Handle(req Request) Response {
	// Validate request
	if req.Method == "" {
		return MakeErrorResponse(req.ID, NewInvalidRequestError("method is required"))
	}

	// Route based on method prefix
	parts := strings.SplitN(req.Method, ".", 2)
	if len(parts) != 2 {
		return MakeErrorResponse(req.ID, NewMethodNotFoundError(req.Method))
	}

	resource := parts[0]
	action := parts[1]

	switch resource {
	case "ticket":
		return h.handleTicket(req, action)
	case "project":
		return h.handleProject(req, action)
	case "workflow":
		return h.handleWorkflow(req, action)
	case "phase":
		return h.handlePhase(req, action)
	case "findings":
		return h.handleFindings(req, action)
	case "agent":
		return h.handleAgent(req, action)
	default:
		return MakeErrorResponse(req.ID, NewMethodNotFoundError(req.Method))
	}
}

func (h *Handler) handleTicket(req Request, action string) Response {
	projectID := req.Project
	if projectID == "" {
		return MakeErrorResponse(req.ID, NewValidationError("project is required"))
	}

	switch action {
	case "create":
		var params types.TicketCreateRequest
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return MakeErrorResponse(req.ID, NewInvalidParamsError(err.Error()))
		}
		if params.Title == "" {
			return MakeErrorResponse(req.ID, NewValidationError("title is required"))
		}
		ticket, err := h.ticketSvc.Create(projectID, &params)
		if err != nil {
			return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
		}
		return MakeResponse(req.ID, ticket)

	case "get":
		var params struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return MakeErrorResponse(req.ID, NewInvalidParamsError(err.Error()))
		}
		ticket, err := h.ticketSvc.Get(projectID, params.ID)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				return MakeErrorResponse(req.ID, NewNotFoundError(err.Error()))
			}
			return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
		}
		return MakeResponse(req.ID, ticket)

	case "list":
		var params types.TicketListRequest
		if req.Params != nil {
			if err := json.Unmarshal(req.Params, &params); err != nil {
				return MakeErrorResponse(req.ID, NewInvalidParamsError(err.Error()))
			}
		}
		tickets, err := h.ticketSvc.List(projectID, &params)
		if err != nil {
			return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
		}
		return MakeResponse(req.ID, tickets)

	case "update":
		var params struct {
			ID string `json:"id"`
			types.TicketUpdateRequest
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return MakeErrorResponse(req.ID, NewInvalidParamsError(err.Error()))
		}
		if err := h.ticketSvc.Update(projectID, params.ID, &params.TicketUpdateRequest); err != nil {
			if strings.Contains(err.Error(), "not found") {
				return MakeErrorResponse(req.ID, NewNotFoundError(err.Error()))
			}
			return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
		}
		return MakeResponse(req.ID, map[string]string{"status": "updated"})

	case "close":
		var params struct {
			ID     string `json:"id"`
			Reason string `json:"reason,omitempty"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return MakeErrorResponse(req.ID, NewInvalidParamsError(err.Error()))
		}
		if err := h.ticketSvc.Close(projectID, params.ID, params.Reason); err != nil {
			if strings.Contains(err.Error(), "not found") {
				return MakeErrorResponse(req.ID, NewNotFoundError(err.Error()))
			}
			return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
		}
		return MakeResponse(req.ID, map[string]string{"status": "closed"})

	case "delete":
		var params struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return MakeErrorResponse(req.ID, NewInvalidParamsError(err.Error()))
		}
		if err := h.ticketSvc.Delete(projectID, params.ID); err != nil {
			if strings.Contains(err.Error(), "not found") {
				return MakeErrorResponse(req.ID, NewNotFoundError(err.Error()))
			}
			return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
		}
		return MakeResponse(req.ID, map[string]string{"status": "deleted"})

	case "search":
		var params types.TicketSearchRequest
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return MakeErrorResponse(req.ID, NewInvalidParamsError(err.Error()))
		}
		tickets, err := h.ticketSvc.Search(projectID, params.Query)
		if err != nil {
			return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
		}
		return MakeResponse(req.ID, tickets)

	case "ready":
		tickets, err := h.ticketSvc.GetReady(projectID)
		if err != nil {
			return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
		}
		return MakeResponse(req.ID, tickets)

	case "status":
		var params types.StatusRequest
		if req.Params != nil {
			if err := json.Unmarshal(req.Params, &params); err != nil {
				return MakeErrorResponse(req.ID, NewInvalidParamsError(err.Error()))
			}
		}
		if params.PendingLimit <= 0 {
			params.PendingLimit = 20
		}
		if params.CompletedLimit <= 0 {
			params.CompletedLimit = 15
		}
		status, err := h.ticketSvc.GetStatus(projectID, params.PendingLimit, params.CompletedLimit)
		if err != nil {
			return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
		}
		return MakeResponse(req.ID, status)

	case "dep.add":
		var params types.DependencyRequest
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return MakeErrorResponse(req.ID, NewInvalidParamsError(err.Error()))
		}
		if err := h.ticketSvc.AddDependency(projectID, params.Child, params.Parent); err != nil {
			return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
		}
		return MakeResponse(req.ID, map[string]string{"status": "added"})

	case "dep.remove":
		var params types.DependencyRequest
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return MakeErrorResponse(req.ID, NewInvalidParamsError(err.Error()))
		}
		if err := h.ticketSvc.RemoveDependency(projectID, params.Child, params.Parent); err != nil {
			return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
		}
		return MakeResponse(req.ID, map[string]string{"status": "removed"})

	default:
		return MakeErrorResponse(req.ID, NewMethodNotFoundError("ticket."+action))
	}
}

func (h *Handler) handleProject(req Request, action string) Response {
	switch action {
	case "create":
		var params struct {
			ID string `json:"id"`
			types.ProjectCreateRequest
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return MakeErrorResponse(req.ID, NewInvalidParamsError(err.Error()))
		}
		if params.ID == "" {
			return MakeErrorResponse(req.ID, NewValidationError("id is required"))
		}
		project, err := h.projectSvc.Create(params.ID, &params.ProjectCreateRequest)
		if err != nil {
			if strings.Contains(err.Error(), "already exists") {
				return MakeErrorResponse(req.ID, NewConflictError(err.Error()))
			}
			return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
		}
		return MakeResponse(req.ID, project)

	case "get":
		var params struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return MakeErrorResponse(req.ID, NewInvalidParamsError(err.Error()))
		}
		project, err := h.projectSvc.Get(params.ID)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				return MakeErrorResponse(req.ID, NewNotFoundError(err.Error()))
			}
			return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
		}
		return MakeResponse(req.ID, project)

	case "list":
		projects, err := h.projectSvc.List()
		if err != nil {
			return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
		}
		return MakeResponse(req.ID, projects)

	case "delete":
		var params struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return MakeErrorResponse(req.ID, NewInvalidParamsError(err.Error()))
		}
		if err := h.projectSvc.Delete(params.ID); err != nil {
			if strings.Contains(err.Error(), "not found") {
				return MakeErrorResponse(req.ID, NewNotFoundError(err.Error()))
			}
			return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
		}
		return MakeResponse(req.ID, map[string]string{"status": "deleted"})

	default:
		return MakeErrorResponse(req.ID, NewMethodNotFoundError("project."+action))
	}
}

func (h *Handler) handleWorkflow(req Request, action string) Response {
	projectID := req.Project
	if projectID == "" {
		return MakeErrorResponse(req.ID, NewValidationError("project is required"))
	}

	projectRoot := h.getProjectRoot(projectID)

	switch action {
	case "init":
		var params struct {
			TicketID string `json:"ticket_id"`
			types.WorkflowInitRequest
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return MakeErrorResponse(req.ID, NewInvalidParamsError(err.Error()))
		}
		if err := h.workflowSvc.Init(projectID, params.TicketID, &params.WorkflowInitRequest, projectRoot); err != nil {
			if strings.Contains(err.Error(), "not found") {
				return MakeErrorResponse(req.ID, NewNotFoundError(err.Error()))
			}
			if strings.Contains(err.Error(), "already initialized") {
				return MakeErrorResponse(req.ID, NewConflictError(err.Error()))
			}
			return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
		}
		return MakeResponse(req.ID, map[string]string{"status": "initialized"})

	case "status", "get":
		var params struct {
			TicketID string `json:"ticket_id"`
			types.WorkflowGetRequest
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return MakeErrorResponse(req.ID, NewInvalidParamsError(err.Error()))
		}
		status, err := h.workflowSvc.GetStatus(projectID, params.TicketID, &params.WorkflowGetRequest, projectRoot)
		if err != nil {
			if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "not initialized") {
				return MakeErrorResponse(req.ID, NewNotFoundError(err.Error()))
			}
			return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
		}
		return MakeResponse(req.ID, status)

	case "set":
		var params struct {
			TicketID string `json:"ticket_id"`
			types.WorkflowSetRequest
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return MakeErrorResponse(req.ID, NewInvalidParamsError(err.Error()))
		}
		if err := h.workflowSvc.Set(projectID, params.TicketID, &params.WorkflowSetRequest); err != nil {
			if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "not initialized") {
				return MakeErrorResponse(req.ID, NewNotFoundError(err.Error()))
			}
			return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
		}
		return MakeResponse(req.ID, map[string]string{"status": "updated"})

	case "list":
		workflows, err := h.workflowSvc.ListWorkflows(projectRoot)
		if err != nil {
			return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
		}
		return MakeResponse(req.ID, workflows)

	default:
		return MakeErrorResponse(req.ID, NewMethodNotFoundError("workflow."+action))
	}
}

func (h *Handler) handlePhase(req Request, action string) Response {
	projectID := req.Project
	if projectID == "" {
		return MakeErrorResponse(req.ID, NewValidationError("project is required"))
	}

	switch action {
	case "start":
		var params struct {
			TicketID string `json:"ticket_id"`
			types.PhaseUpdateRequest
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return MakeErrorResponse(req.ID, NewInvalidParamsError(err.Error()))
		}
		if err := h.workflowSvc.StartPhase(projectID, params.TicketID, &params.PhaseUpdateRequest); err != nil {
			if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "not initialized") {
				return MakeErrorResponse(req.ID, NewNotFoundError(err.Error()))
			}
			return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
		}
		return MakeResponse(req.ID, map[string]string{"status": "started"})

	case "complete":
		var params struct {
			TicketID string `json:"ticket_id"`
			types.PhaseUpdateRequest
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return MakeErrorResponse(req.ID, NewInvalidParamsError(err.Error()))
		}
		if err := h.workflowSvc.CompletePhase(projectID, params.TicketID, &params.PhaseUpdateRequest); err != nil {
			if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "not initialized") {
				return MakeErrorResponse(req.ID, NewNotFoundError(err.Error()))
			}
			return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
		}
		return MakeResponse(req.ID, map[string]string{"status": "completed"})

	default:
		return MakeErrorResponse(req.ID, NewMethodNotFoundError("phase."+action))
	}
}

func (h *Handler) handleFindings(req Request, action string) Response {
	projectID := req.Project
	if projectID == "" {
		return MakeErrorResponse(req.ID, NewValidationError("project is required"))
	}

	switch action {
	case "add":
		var params struct {
			TicketID string `json:"ticket_id"`
			types.FindingsAddRequest
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return MakeErrorResponse(req.ID, NewInvalidParamsError(err.Error()))
		}
		if err := h.findingsSvc.Add(projectID, params.TicketID, &params.FindingsAddRequest); err != nil {
			if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "not initialized") {
				return MakeErrorResponse(req.ID, NewNotFoundError(err.Error()))
			}
			return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
		}
		return MakeResponse(req.ID, map[string]string{"status": "added"})

	case "add-bulk":
		var params struct {
			TicketID string `json:"ticket_id"`
			types.FindingsAddBulkRequest
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return MakeErrorResponse(req.ID, NewInvalidParamsError(err.Error()))
		}
		if err := h.findingsSvc.AddBulk(projectID, params.TicketID, &params.FindingsAddBulkRequest); err != nil {
			if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "not initialized") {
				return MakeErrorResponse(req.ID, NewNotFoundError(err.Error()))
			}
			return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
		}
		return MakeResponse(req.ID, map[string]interface{}{
			"status": "added",
			"count":  len(params.KeyValues),
		})

	case "get":
		var params struct {
			TicketID string `json:"ticket_id"`
			types.FindingsGetRequest
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return MakeErrorResponse(req.ID, NewInvalidParamsError(err.Error()))
		}
		findings, err := h.findingsSvc.Get(projectID, params.TicketID, &params.FindingsGetRequest)
		if err != nil {
			if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "not initialized") {
				return MakeErrorResponse(req.ID, NewNotFoundError(err.Error()))
			}
			return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
		}
		return MakeResponse(req.ID, findings)

	default:
		return MakeErrorResponse(req.ID, NewMethodNotFoundError("findings."+action))
	}
}

func (h *Handler) handleAgent(req Request, action string) Response {
	projectID := req.Project

	switch action {
	case "list":
		projectRoot := "."
		if projectID != "" {
			projectRoot = h.getProjectRoot(projectID)
		}
		agents, err := h.agentSvc.ListAgentTypes(projectRoot)
		if err != nil {
			return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
		}
		return MakeResponse(req.ID, agents)

	case "active":
		if projectID == "" {
			return MakeErrorResponse(req.ID, NewValidationError("project is required"))
		}
		var params struct {
			TicketID string `json:"ticket_id"`
			types.AgentActiveRequest
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return MakeErrorResponse(req.ID, NewInvalidParamsError(err.Error()))
		}
		active, err := h.agentSvc.GetActive(projectID, params.TicketID, &params.AgentActiveRequest)
		if err != nil {
			return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
		}
		return MakeResponse(req.ID, active)

	case "kill":
		if projectID == "" {
			return MakeErrorResponse(req.ID, NewValidationError("project is required"))
		}
		var params struct {
			TicketID string `json:"ticket_id"`
			types.AgentKillRequest
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return MakeErrorResponse(req.ID, NewInvalidParamsError(err.Error()))
		}
		killed, err := h.agentSvc.Kill(projectID, params.TicketID, &params.AgentKillRequest)
		if err != nil {
			return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
		}
		return MakeResponse(req.ID, map[string]int{"killed": killed})

	case "complete":
		if projectID == "" {
			return MakeErrorResponse(req.ID, NewValidationError("project is required"))
		}
		var params struct {
			TicketID string `json:"ticket_id"`
			types.AgentCompleteRequest
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return MakeErrorResponse(req.ID, NewInvalidParamsError(err.Error()))
		}
		if err := h.agentSvc.Complete(projectID, params.TicketID, &params.AgentCompleteRequest); err != nil {
			return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
		}
		return MakeResponse(req.ID, map[string]string{"status": "completed"})

	case "fail":
		if projectID == "" {
			return MakeErrorResponse(req.ID, NewValidationError("project is required"))
		}
		var params struct {
			TicketID string `json:"ticket_id"`
			types.AgentCompleteRequest
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return MakeErrorResponse(req.ID, NewInvalidParamsError(err.Error()))
		}
		if err := h.agentSvc.Fail(projectID, params.TicketID, &params.AgentCompleteRequest); err != nil {
			return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
		}
		return MakeResponse(req.ID, map[string]string{"status": "failed"})

	case "sessions":
		if projectID == "" {
			return MakeErrorResponse(req.ID, NewValidationError("project is required"))
		}
		var params struct {
			TicketID string `json:"ticket_id,omitempty"`
			Limit    int    `json:"limit,omitempty"`
		}
		if req.Params != nil {
			if err := json.Unmarshal(req.Params, &params); err != nil {
				return MakeErrorResponse(req.ID, NewInvalidParamsError(err.Error()))
			}
		}
		if params.TicketID != "" {
			sessions, err := h.agentSvc.GetTicketSessions(projectID, params.TicketID)
			if err != nil {
				return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
			}
			return MakeResponse(req.ID, sessions)
		}
		sessions, err := h.agentSvc.GetRecentSessions(projectID, params.Limit)
		if err != nil {
			return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
		}
		return MakeResponse(req.ID, sessions)

	// Note: agent.spawn is handled specially with streaming - see spawner integration
	case "spawn", "preview":
		return MakeErrorResponse(req.ID, NewInternalError("spawn and preview require spawner integration - use CLI directly for now"))

	default:
		return MakeErrorResponse(req.ID, NewMethodNotFoundError("agent."+action))
	}
}
