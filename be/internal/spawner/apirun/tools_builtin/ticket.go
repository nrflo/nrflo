package tools_builtin

import (
	"context"
	"encoding/json"
	"strings"

	"be/internal/service"
	"be/internal/spawner/apirun"
	"be/internal/spawner/apirun/provider"
	"be/internal/types"
	"be/internal/ws"
)

// ticketCreateHandler lets a project-scoped agent (e.g. a ticket-creator) create
// a ticket. It mirrors POST /api/v1/tickets: the project is the agent's own
// project, and the new ticket id is returned so the agent can wire dependencies.
type ticketCreateHandler struct{}

func (ticketCreateHandler) Spec() provider.ToolSpec {
	return provider.ToolSpec{
		Name: "ticket_create",
		Description: "Create a ticket in the current project and return its id. " +
			"Use the returned id with ticket_add_dependency to connect tickets.",
		InputSchema: json.RawMessage(`{
"type":"object",
"properties":{
"id":{"type":"string","description":"explicit ticket id (e.g. an external/Jira id); auto-generated when omitted"},
"title":{"type":"string","description":"short, actionable title"},
"description":{"type":"string","description":"full description (markdown ok)"},
"type":{"type":"string","enum":["bug","feature","task","epic"],"description":"defaults to task"},
"priority":{"type":"integer","minimum":1,"maximum":4,"description":"1=critical .. 4=low; defaults to 2"},
"parent_id":{"type":"string","description":"parent epic/ticket id (optional)"}
},
"required":["title"],
"additionalProperties":false
}`),
	}
}

func (ticketCreateHandler) Invoke(ctx context.Context, env apirun.ToolEnv, input json.RawMessage) (string, bool, error) {
	var args struct {
		ID          string `json:"id"`
		Title       string `json:"title"`
		Description string `json:"description"`
		Type        string `json:"type"`
		Priority    int    `json:"priority"`
		ParentID    string `json:"parent_id"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return invalidArgs(err)
	}
	if strings.TrimSpace(args.Title) == "" {
		return "title is required", true, nil
	}
	if env.Ticket == nil {
		return missingService("ticket")
	}
	ticket, err := env.Ticket.Create(env.ProjectID, &types.TicketCreateRequest{
		ID:             args.ID,
		Title:          args.Title,
		Description:    args.Description,
		Type:           args.Type,
		Priority:       args.Priority,
		ParentTicketID: args.ParentID,
	})
	if err != nil {
		return err.Error(), true, nil
	}
	broadcastTicketCreated(env, ticket.ID)
	out, _ := json.Marshal(map[string]string{"ticket_id": ticket.ID, "title": ticket.Title})
	return string(out), false, nil
}

// ticketUpdateHandler applies a partial update to an existing ticket in the
// agent's own project. It mirrors PATCH /api/v1/tickets/{id}: only provided
// fields change.
type ticketUpdateHandler struct{}

func (ticketUpdateHandler) Spec() provider.ToolSpec {
	return provider.ToolSpec{
		Name: "ticket_update",
		Description: "Update fields of an existing ticket in the current project. " +
			"Only the provided fields change; omitted fields keep their current value.",
		InputSchema: json.RawMessage(`{
"type":"object",
"properties":{
"ticket_id":{"type":"string","description":"the ticket to update"},
"title":{"type":"string","description":"new title"},
"description":{"type":"string","description":"new full description (markdown ok)"},
"status":{"type":"string","enum":["open","in_progress","closed"]},
"type":{"type":"string","enum":["bug","feature","task","epic"]},
"priority":{"type":"integer","minimum":1,"maximum":4,"description":"1=critical .. 4=low"}
},
"required":["ticket_id"],
"additionalProperties":false
}`),
	}
}

func (ticketUpdateHandler) Invoke(ctx context.Context, env apirun.ToolEnv, input json.RawMessage) (string, bool, error) {
	var args struct {
		TicketID    string  `json:"ticket_id"`
		Title       *string `json:"title"`
		Description *string `json:"description"`
		Status      *string `json:"status"`
		Type        *string `json:"type"`
		Priority    *int    `json:"priority"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return invalidArgs(err)
	}
	if strings.TrimSpace(args.TicketID) == "" {
		return "ticket_id is required", true, nil
	}
	if args.Title == nil && args.Description == nil && args.Status == nil && args.Type == nil && args.Priority == nil {
		return "at least one field to update is required", true, nil
	}
	if args.Status != nil {
		switch *args.Status {
		case "open", "in_progress", "closed":
		default:
			return "invalid status: must be open, in_progress, or closed", true, nil
		}
	}
	if args.Type != nil {
		switch *args.Type {
		case "bug", "feature", "task", "epic":
		default:
			return "invalid type: must be bug, feature, task, or epic", true, nil
		}
	}
	if args.Priority != nil && (*args.Priority < 1 || *args.Priority > 4) {
		return "invalid priority: must be 1..4", true, nil
	}
	if env.Ticket == nil {
		return missingService("ticket")
	}
	if err := env.Ticket.Update(env.ProjectID, args.TicketID, &types.TicketUpdateRequest{
		Title:       args.Title,
		Description: args.Description,
		Status:      args.Status,
		Type:        args.Type,
		Priority:    args.Priority,
	}); err != nil {
		return err.Error(), true, nil
	}
	updated, err := env.Ticket.Get(env.ProjectID, args.TicketID)
	if err != nil {
		return err.Error(), true, nil
	}
	service.BroadcastFromCtx(env.WSHub, ws.EventTicketUpdated, service.BroadcastCtx{ProjectID: env.ProjectID, TicketID: updated.ID}, map[string]interface{}{
		"status": string(updated.Status),
		"action": "updated",
	})
	out, _ := json.Marshal(map[string]string{"ticket_id": updated.ID, "status": string(updated.Status)})
	return string(out), false, nil
}

// ticketAddDependencyHandler records that ticket_id is blocked by depends_on_id
// (depends_on_id must complete first). Mirrors POST /api/v1/dependencies.
type ticketAddDependencyHandler struct{}

func (ticketAddDependencyHandler) Spec() provider.ToolSpec {
	return provider.ToolSpec{
		Name: "ticket_add_dependency",
		Description: "Add a blocking dependency: ticket_id is blocked by depends_on_id " +
			"(depends_on_id must complete before ticket_id becomes runnable). Both must already exist.",
		InputSchema: json.RawMessage(`{
"type":"object",
"properties":{
"ticket_id":{"type":"string","description":"the blocked ticket"},
"depends_on_id":{"type":"string","description":"the blocker that must complete first"}
},
"required":["ticket_id","depends_on_id"],
"additionalProperties":false
}`),
	}
}

func (ticketAddDependencyHandler) Invoke(ctx context.Context, env apirun.ToolEnv, input json.RawMessage) (string, bool, error) {
	var args struct {
		TicketID    string `json:"ticket_id"`
		DependsOnID string `json:"depends_on_id"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return invalidArgs(err)
	}
	if strings.TrimSpace(args.TicketID) == "" || strings.TrimSpace(args.DependsOnID) == "" {
		return "ticket_id and depends_on_id are required", true, nil
	}
	if env.Ticket == nil {
		return missingService("ticket")
	}
	if err := env.Ticket.AddDependency(env.ProjectID, args.TicketID, args.DependsOnID, env.AgentType); err != nil {
		return err.Error(), true, nil
	}
	return "ok", false, nil
}

func broadcastTicketCreated(env apirun.ToolEnv, ticketID string) {
	service.BroadcastFromCtx(env.WSHub, ws.EventTicketUpdated, service.BroadcastCtx{ProjectID: env.ProjectID, TicketID: ticketID}, map[string]interface{}{
		"status": "open",
		"action": "created",
	})
}
