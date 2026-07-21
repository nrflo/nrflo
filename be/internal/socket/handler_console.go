package socket

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"be/internal/console"
	"be/internal/logger"
	"be/internal/service"
)

// handleConsole serves console-session lifecycle over the trusted Unix socket.
// Filesystem access to the socket IS the authorization: a local caller
// (`nrflo_server console` against a loopback server) already holds full access
// to $NRFLO_HOME, so it mints a console session here instead of exchanging a
// service token over HTTP. Remote callers can't reach this socket and keep the
// token path.
func (h *Handler) handleConsole(ctx context.Context, req Request, action string) Response {
	switch action {
	case "session":
		return h.handleConsoleSession(ctx, req)
	case "chat":
		return h.handleConsoleChat(ctx, req)
	case "attach":
		return h.handleConsoleAttach(ctx, req)
	case "close":
		return h.handleConsoleClose(ctx, req)
	case "catalog":
		return h.handleConsoleCatalog(ctx, req)
	default:
		logger.Warn(ctx, "unknown socket method", "method", "console."+action)
		return MakeErrorResponse(req.ID, NewMethodNotFoundError("console."+action))
	}
}

func (h *Handler) handleConsoleCatalog(ctx context.Context, req Request) Response {
	if h.consoleChat == nil {
		return MakeErrorResponse(req.ID, NewInternalError("console chat service unavailable"))
	}
	var params struct {
		Project string `json:"project"`
		Cwd     string `json:"cwd"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return MakeErrorResponse(req.ID, NewInvalidParamsError(err.Error()))
	}
	projectID := h.resolveConsoleProject(ctx, strings.TrimSpace(params.Project), params.Cwd)
	catalog, err := h.consoleChat.Catalog(projectID)
	if err != nil {
		if errors.Is(err, service.ErrConsoleProjectNotFound) {
			return MakeErrorResponse(req.ID, NewNotFoundError("project not found: "+projectID))
		}
		return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
	}
	return MakeResponse(req.ID, catalog)
}

func (h *Handler) handleConsoleAttach(ctx context.Context, req Request) Response {
	if h.consoleChat == nil {
		return MakeErrorResponse(req.ID, NewInternalError("console chat service unavailable"))
	}
	var params struct {
		Project   string `json:"project"`
		Cwd       string `json:"cwd"`
		SessionID string `json:"session_id"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return MakeErrorResponse(req.ID, NewInvalidParamsError(err.Error()))
	}
	sid := strings.TrimSpace(params.SessionID)
	if sid == "" {
		return MakeErrorResponse(req.ID, NewValidationError("session_id is required"))
	}
	projectID := h.resolveConsoleProject(ctx, strings.TrimSpace(params.Project), params.Cwd)
	token, err := h.consoleChat.AttachAuthenticated(sid, projectID)
	if err != nil {
		switch {
		case errors.Is(err, console.ErrChatSessionNotFound):
			return MakeErrorResponse(req.ID, NewNotFoundError("live console chat not found: "+sid))
		case errors.Is(err, console.ErrChatProjectMismatch):
			return MakeErrorResponse(req.ID, NewValidationError("console chat belongs to another project"))
		default:
			return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
		}
	}
	return MakeResponse(req.ID, map[string]string{
		"session_id": sid, "token": token, "project_id": projectID,
	})
}

func (h *Handler) handleConsoleClose(ctx context.Context, req Request) Response {
	if h.consoleChat == nil {
		return MakeErrorResponse(req.ID, NewInternalError("console chat service unavailable"))
	}
	var params struct {
		Project   string `json:"project"`
		Cwd       string `json:"cwd"`
		SessionID string `json:"session_id"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return MakeErrorResponse(req.ID, NewInvalidParamsError(err.Error()))
	}
	sid := strings.TrimSpace(params.SessionID)
	if sid == "" {
		return MakeErrorResponse(req.ID, NewValidationError("session_id is required"))
	}
	projectID := h.resolveConsoleProject(ctx, strings.TrimSpace(params.Project), params.Cwd)
	if err := h.consoleChat.CloseAuthenticated(sid, projectID); err != nil {
		switch {
		case errors.Is(err, console.ErrChatSessionNotFound):
			return MakeErrorResponse(req.ID, NewNotFoundError("live console chat not found: "+sid))
		case errors.Is(err, console.ErrChatProjectMismatch):
			return MakeErrorResponse(req.ID, NewValidationError("console chat belongs to another project"))
		default:
			return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
		}
	}
	return MakeResponse(req.ID, map[string]string{"session_id": sid})
}

func (h *Handler) handleConsoleChat(ctx context.Context, req Request) Response {
	if h.consoleChat == nil {
		return MakeErrorResponse(req.ID, NewInternalError("console chat service unavailable"))
	}
	var params struct {
		Project          string `json:"project"`
		Cwd              string `json:"cwd"`
		Engine           string `json:"engine"`
		Model            string `json:"model"`
		Effort           string `json:"reasoning_effort"`
		SystemTemplateID string `json:"system_template_id"`
		Profile          string `json:"profile"`
		RefineryEnabled  bool   `json:"refinery_enabled"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return MakeErrorResponse(req.ID, NewInvalidParamsError(err.Error()))
	}
	engine := strings.TrimSpace(params.Engine)
	modelID := strings.TrimSpace(params.Model)
	if engine == "" {
		return MakeErrorResponse(req.ID, NewValidationError("engine is required"))
	}
	projectID := h.resolveConsoleProject(ctx, strings.TrimSpace(params.Project), params.Cwd)
	sid, token, err := h.consoleChat.CreateAuthenticated(engine, modelID, strings.TrimSpace(params.Effort), projectID, strings.TrimSpace(params.SystemTemplateID), strings.TrimSpace(params.Profile), params.RefineryEnabled)
	if err != nil {
		if errors.Is(err, service.ErrConsoleProjectNotFound) {
			return MakeErrorResponse(req.ID, NewNotFoundError("project not found: "+projectID))
		}
		if errors.Is(err, console.ErrUnknownProfile) {
			return MakeErrorResponse(req.ID, NewValidationError(err.Error()))
		}
		return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
	}
	logger.Info(ctx, "console chat minted over socket", "session_id", sid, "project", projectID, "engine", engine)
	return MakeResponse(req.ID, map[string]string{
		"session_id": sid,
		"token":      token,
		"project_id": projectID,
		"engine":     engine,
		"model":      modelID,
	})
}

// handleConsoleSession mints a kind='console' agent_sessions row and returns its
// bearer token once. The project resolves server-side: an explicit hint (the
// caller's --project / NRFLO_PROJECT) wins, else the caller's cwd is matched
// against project root_paths, else the hidden global project. The ticket_id hint
// (the caller's git branch) is validated against the resolved project and
// dropped silently when it is not a known ticket — mirroring the HTTP
// POST /api/v1/console/sessions contract.
func (h *Handler) handleConsoleSession(ctx context.Context, req Request) Response {
	var params struct {
		Project  string `json:"project"`
		Cwd      string `json:"cwd"`
		TicketID string `json:"ticket_id"`
	}
	if len(req.Params) > 0 {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return MakeErrorResponse(req.ID, NewInvalidParamsError(err.Error()))
		}
	}

	projectID := h.resolveConsoleProject(ctx, strings.TrimSpace(params.Project), params.Cwd)

	// Validate the git-branch ticket hint against the resolved project; an
	// unknown branch is not a ticket, so drop it (never an error).
	ticketID := strings.TrimSpace(params.TicketID)
	if ticketID != "" {
		if _, gerr := service.NewTicketService(h.pool, h.clk).Get(projectID, ticketID); gerr != nil {
			ticketID = ""
		}
	}

	consoleSvc := service.NewConsoleService(h.pool, h.clk)
	sessionID, token, err := consoleSvc.CreateSession(projectID, ticketID)
	if err != nil {
		if errors.Is(err, service.ErrConsoleProjectNotFound) {
			return MakeErrorResponse(req.ID, NewNotFoundError("project not found: "+projectID))
		}
		logger.Error(ctx, "socket handler error", "method", req.Method, "error", err)
		return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
	}

	logger.Info(ctx, "console session minted over socket", "session_id", sessionID, "project", projectID)
	return MakeResponse(req.ID, map[string]string{
		"session_id": sessionID,
		"token":      token,
		"project_id": projectID,
		"ticket_id":  ticketID,
	})
}

// resolveConsoleProject picks the project a socket-minted console session is
// scoped to: an explicit hint wins verbatim (CreateSession errors if it does
// not exist), else the caller's cwd is matched against project root_paths, else
// the hidden global project. A cwd-match failure degrades to the global project
// — a bad cwd never produces a wrong match (the prefix check fails closed).
func (h *Handler) resolveConsoleProject(ctx context.Context, projectHint, cwd string) string {
	if projectHint != "" {
		return projectHint
	}
	if cwd != "" {
		if id, err := h.projectSvc.ResolveByCwd(cwd); err != nil {
			logger.Warn(ctx, "console cwd project resolution failed", "error", err)
		} else if id != "" {
			return id
		}
	}
	return service.GlobalProjectID
}
