package socket

import (
	"context"
	"encoding/json"

	"be/internal/repo"
)

// rawFindingStr extracts a string value from a raw findings map.
func rawFindingStr(raw map[string]json.RawMessage, key string) string {
	v, ok := raw[key]
	if !ok {
		return ""
	}
	var s string
	json.Unmarshal(v, &s) //nolint:errcheck
	return s
}

func (h *Handler) handleScript(ctx context.Context, req Request, action string) Response {
	switch action {
	case "context":
		return h.handleScriptContext(ctx, req)
	default:
		return MakeErrorResponse(req.ID, NewMethodNotFoundError("script."+action))
	}
}

func (h *Handler) handleScriptContext(_ context.Context, req Request) Response {
	var params struct {
		SessionID string `json:"session_id"`
	}
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return MakeErrorResponse(req.ID, NewInvalidParamsError(err.Error()))
		}
	}
	if params.SessionID == "" {
		return MakeErrorResponse(req.ID, NewValidationError("session_id is required"))
	}

	asRepo := repo.NewAgentSessionRepo(h.pool, h.clk)
	session, err := asRepo.Get(params.SessionID)
	if err != nil {
		return MakeErrorResponse(req.ID, NewNotFoundError(err.Error()))
	}

	wfiRepo := repo.NewWorkflowInstanceRepo(h.pool, h.clk)
	wfi, err := wfiRepo.Get(session.WorkflowInstanceID)
	if err != nil {
		return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
	}

	findingRepo := repo.NewFindingRepo(h.pool, h.clk)
	wfiRaw, _ := findingRepo.GetOwn("workflow_instance", wfi.ID)
	sessionRaw, _ := findingRepo.GetOwn("session", params.SessionID)

	userInstructions := rawFindingStr(wfiRaw, "user_instructions")

	var callbackInfo interface{}
	if cbRaw, ok := wfiRaw["_callback"]; ok {
		var m map[string]interface{}
		if json.Unmarshal(cbRaw, &m) == nil {
			if instr, ok := m["instructions"].(string); ok && instr != "" {
				callbackInfo = map[string]interface{}{
					"instructions": instr,
					"from_agent":   m["from_agent"],
					"level":        m["level"],
				}
			}
		}
	}

	previousData := rawFindingStr(sessionRaw, "to_resume")

	ticketID := session.TicketID
	ticketTitle := ""
	ticketDescription := ""
	if ticketID != "" {
		ticketRepo := repo.NewTicketRepo(h.pool, h.clk)
		if ticket, err := ticketRepo.Get(session.ProjectID, ticketID); err == nil {
			ticketTitle = ticket.Title
			if ticket.Description.Valid {
				ticketDescription = ticket.Description.String
			}
		}
	}

	return MakeResponse(req.ID, map[string]interface{}{
		"session_id":          session.ID,
		"instance_id":         wfi.ID,
		"project_id":          session.ProjectID,
		"agent_type":          session.AgentType,
		"workflow_id":         wfi.WorkflowID,
		"scope_type":          wfi.ScopeType,
		"ticket_id":           ticketID,
		"ticket_title":        ticketTitle,
		"ticket_description":  ticketDescription,
		"user_instructions":   userInstructions,
		"callback":            callbackInfo,
		"previous_data":       previousData,
	})
}
