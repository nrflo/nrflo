package socket

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"

	"be/internal/logger"
	"be/internal/repo"
	"be/internal/service"
	"be/internal/spawner"
	"be/internal/ws"
)

// resolveSessionContext queries project_id and workflow_instance_id for a session.
func (h *Handler) resolveSessionContext(sessionID string) (projectID, wfiID string, err error) {
	if sessionID == "" {
		return "", "", newValidationErr("session_id is required")
	}
	err = h.pool.QueryRow(
		`SELECT project_id, workflow_instance_id FROM agent_sessions WHERE id = ?`, sessionID,
	).Scan(&projectID, &wfiID)
	if err != nil {
		return "", "", newNotFoundErr("session " + sessionID + " not found")
	}
	return projectID, wfiID, nil
}

func newValidationErr(msg string) error  { return &rpcErr{NewValidationError(msg)} }
func newNotFoundErr(msg string) error    { return &rpcErr{NewNotFoundError(msg)} }

type rpcErr struct{ info *ErrorInfo }

func (e *rpcErr) Error() string { return e.info.Message }

func (h *Handler) handleArtifact(ctx context.Context, req Request, action string) Response {
	switch action {
	case "add":
		return h.handleArtifactAdd(ctx, req)
	case "list":
		return h.handleArtifactList(ctx, req)
	case "get":
		return h.handleArtifactGet(ctx, req)
	default:
		logger.Warn(ctx, "unknown socket method", "method", "artifact."+action)
		return MakeErrorResponse(req.ID, NewMethodNotFoundError("artifact."+action))
	}
}

func (h *Handler) handleArtifactAdd(ctx context.Context, req Request) Response {
	var params struct {
		SessionID   string `json:"session_id"`
		Name        string `json:"name"`
		ContentB64  string `json:"content_b64"`
		ContentType string `json:"content_type"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return MakeErrorResponse(req.ID, NewInvalidParamsError(err.Error()))
	}
	if params.SessionID == "" {
		return MakeErrorResponse(req.ID, NewValidationError("session_id is required"))
	}
	if params.Name == "" {
		return MakeErrorResponse(req.ID, NewValidationError("name is required"))
	}
	if params.ContentB64 == "" {
		return MakeErrorResponse(req.ID, NewValidationError("content_b64 is required"))
	}

	data, err := base64.StdEncoding.DecodeString(params.ContentB64)
	if err != nil {
		return MakeErrorResponse(req.ID, NewValidationError("invalid base64 content_b64: "+err.Error()))
	}
	const maxBytes = 32 * 1024 * 1024
	if len(data) > maxBytes {
		return MakeErrorResponse(req.ID, NewValidationError("artifact too large: max 32 MiB"))
	}

	projectID, wfiID, err := h.resolveSessionContext(params.SessionID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return MakeErrorResponse(req.ID, NewNotFoundError(err.Error()))
		}
		return MakeErrorResponse(req.ID, NewValidationError(err.Error()))
	}

	a, err := h.artifactSvc.AddFromAgent(ctx, params.SessionID, projectID, wfiID, params.Name, params.ContentType, data)
	if err != nil {
		logger.Error(ctx, "artifact.add failed", "error", err)
		return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
	}

	service.BroadcastFromCtx(h.wsHub, ws.EventArtifactCreated, service.BroadcastCtx{ProjectID: projectID}, map[string]interface{}{
		"artifact_id":          a.ID,
		"workflow_instance_id": wfiID,
		"name":                 a.Name,
	})

	return MakeResponse(req.ID, map[string]string{"id": a.ID, "name": a.Name})
}

func (h *Handler) handleArtifactList(ctx context.Context, req Request) Response {
	var params struct {
		SessionID string `json:"session_id"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return MakeErrorResponse(req.ID, NewInvalidParamsError(err.Error()))
	}
	if params.SessionID == "" {
		return MakeErrorResponse(req.ID, NewValidationError("session_id is required"))
	}

	_, wfiID, err := h.resolveSessionContext(params.SessionID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return MakeErrorResponse(req.ID, NewNotFoundError(err.Error()))
		}
		return MakeErrorResponse(req.ID, NewValidationError(err.Error()))
	}

	artifacts, err := h.artifactSvc.List(ctx, wfiID)
	if err != nil {
		logger.Error(ctx, "artifact.list failed", "error", err)
		return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
	}

	type item struct {
		Name        string `json:"name"`
		SizeBytes   int64  `json:"size_bytes"`
		ContentType string `json:"content_type,omitempty"`
		Source      string `json:"source"`
	}
	result := make([]item, 0, len(artifacts))
	for _, a := range artifacts {
		result = append(result, item{
			Name:        a.Name,
			SizeBytes:   a.SizeBytes,
			ContentType: a.ContentType,
			Source:      a.Source,
		})
	}
	return MakeResponse(req.ID, result)
}

func (h *Handler) handleArtifactGet(ctx context.Context, req Request) Response {
	var params struct {
		SessionID string `json:"session_id"`
		Name      string `json:"name"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return MakeErrorResponse(req.ID, NewInvalidParamsError(err.Error()))
	}
	if params.SessionID == "" {
		return MakeErrorResponse(req.ID, NewValidationError("session_id is required"))
	}
	if params.Name == "" {
		return MakeErrorResponse(req.ID, NewValidationError("name is required"))
	}

	projectID, wfiID, err := h.resolveSessionContext(params.SessionID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return MakeErrorResponse(req.ID, NewNotFoundError(err.Error()))
		}
		return MakeErrorResponse(req.ID, NewValidationError(err.Error()))
	}

	stageDir, err := spawner.EnsureStageDir(projectID, wfiID)
	if err != nil {
		return MakeErrorResponse(req.ID, NewInternalError("stage dir: "+err.Error()))
	}

	storage, err := h.artifactSvc.GetStorage(ctx, projectID)
	if err != nil {
		return MakeErrorResponse(req.ID, NewInternalError("storage: "+err.Error()))
	}

	artifactRepo := repo.NewArtifactRepo(h.pool, h.clk)
	artifacts, err := artifactRepo.List(wfiID)
	if err != nil {
		return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
	}

	for _, a := range artifacts {
		if a.Name == params.Name {
			absPath, matErr := spawner.Materialize(ctx, a, stageDir, storage)
			if matErr != nil {
				return MakeErrorResponse(req.ID, NewInternalError("materialize: "+matErr.Error()))
			}
			return MakeResponse(req.ID, map[string]string{"path": absPath})
		}
	}

	return MakeErrorResponse(req.ID, NewNotFoundError("artifact not found: "+params.Name))
}
