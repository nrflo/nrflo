package socket

import (
	"context"
	"encoding/json"
	"strings"

	"be/internal/logger"
	"be/internal/model"
	"be/internal/repo"
)

// scopeLevel maps observer scope names to numeric precedence: workflow < project < global.
var scopeLevel = map[string]int{"workflow": 1, "project": 2, "global": 3}

// methodSpec describes the authorization requirements for an observer method.
type methodSpec struct {
	scope  string
	mutate bool
}

// observerMethods maps the full dotted action (namespace.subaction) to its spec.
var observerMethods = map[string]methodSpec{
	"workflow.show":           {scope: "workflow"},
	"workflow.runs":           {scope: "workflow"},
	"workflow.findings":       {scope: "workflow"},
	"workflow.logs":           {scope: "workflow"},
	"workflow.trigger":        {scope: "workflow", mutate: true},
	"workflow.retry_failed":   {scope: "workflow", mutate: true},
	"workflow.def.update":     {scope: "workflow", mutate: true},
	"project.workflows":       {scope: "project"},
	"project.runs":            {scope: "project"},
	"project.findings":        {scope: "project"},
	"project.env.list":        {scope: "project"},
	"project.env.set":         {scope: "project", mutate: true},
	"project.env.unset":       {scope: "project", mutate: true},
	"project.workflow.create": {scope: "project", mutate: true},
	"project.workflow.delete": {scope: "project", mutate: true},
	"global.projects":         {scope: "global"},
	"global.recent_sessions":  {scope: "global"},
	"global.health":           {scope: "global"},
	"global.project.create":   {scope: "global", mutate: true},
	"global.project.delete":   {scope: "global", mutate: true},
}

// observerBaseParams carries fields common to all observer method requests.
type observerBaseParams struct {
	SessionID  string `json:"session_id"`
	ProjectID  string `json:"project_id"`
	WorkflowID string `json:"workflow_id"`
}

// authorizeObserver loads the calling session, validates kind=observer, enforces scope
// precedence and same-resource constraints, and (for mutate methods) verifies
// experimental_observer_enabled. Returns the session or an ErrorInfo.
func (h *Handler) authorizeObserver(_ context.Context, sessionID, projectID, workflowID string, spec methodSpec) (*model.AgentSession, *ErrorInfo) {
	if sessionID == "" {
		return nil, NewValidationError("session_id is required")
	}
	asRepo := repo.NewAgentSessionRepo(h.pool, h.clk)
	session, err := asRepo.Get(sessionID)
	if err != nil {
		return nil, NewNotFoundError("session not found: " + sessionID)
	}
	if session.Kind != "observer" {
		return nil, NewValidationError("permission denied: not an observer session")
	}

	observerScope := ""
	if session.ObserverScope.Valid {
		observerScope = session.ObserverScope.String
	}
	if scopeLevel[observerScope] < scopeLevel[spec.scope] {
		return nil, NewValidationError("permission denied: out-of-scope call")
	}

	// Project-scoped observer: target project_id must match the observer's own project.
	if observerScope == "project" && projectID != "" && projectID != session.ProjectID {
		return nil, NewValidationError("permission denied: project_id mismatch")
	}

	// Workflow-scoped observer: target workflow_id must match the observer's workflow instance.
	if observerScope == "workflow" && workflowID != "" {
		wfiRepo := repo.NewWorkflowInstanceRepo(h.pool, h.clk)
		wfi, wfiErr := wfiRepo.Get(session.WorkflowInstanceID)
		if wfiErr != nil {
			return nil, NewInternalError("failed to load workflow instance")
		}
		if workflowID != wfi.WorkflowID {
			return nil, NewValidationError("permission denied: workflow_id mismatch")
		}
	}

	if spec.mutate {
		enabled, gErr := h.globalSettingsSvc.GetExperimentalObserverEnabled()
		if gErr != nil {
			return nil, NewInternalError("failed to check observer feature flag")
		}
		if !enabled {
			return nil, NewValidationError("permission denied: observer feature is disabled")
		}
	}
	return session, nil
}

// handleObserver dispatches observer.* namespace requests.
// action is the part after "observer." (e.g. "workflow.show", "project.env.list").
func (h *Handler) handleObserver(ctx context.Context, req Request, action string) Response {
	parts := strings.SplitN(action, ".", 2)
	if len(parts) != 2 {
		return MakeErrorResponse(req.ID, NewMethodNotFoundError("observer."+action))
	}
	namespace, subAction := parts[0], parts[1]

	var base observerBaseParams
	if req.Params != nil {
		json.Unmarshal(req.Params, &base) //nolint:errcheck
	}

	spec, ok := observerMethods[namespace+"."+subAction]
	if !ok {
		logger.Warn(ctx, "unknown observer method", "method", "observer."+action)
		return MakeErrorResponse(req.ID, NewMethodNotFoundError("observer."+action))
	}

	session, authErr := h.authorizeObserver(ctx, base.SessionID, base.ProjectID, base.WorkflowID, spec)
	if authErr != nil {
		return MakeErrorResponse(req.ID, authErr)
	}

	switch namespace {
	case "workflow":
		return h.handleObserverWorkflow(ctx, req, subAction, session, base)
	case "project":
		return h.handleObserverProject(ctx, req, subAction, session, base)
	case "global":
		return h.handleObserverGlobal(ctx, req, subAction, session, base)
	default:
		return MakeErrorResponse(req.ID, NewMethodNotFoundError("observer."+action))
	}
}
