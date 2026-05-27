package tools_builtin

import (
	"context"
	"encoding/json"
	"strings"

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
	if env.WSHub == nil {
		return
	}
	env.WSHub.Broadcast(ws.NewEvent(ws.EventTicketUpdated, env.ProjectID, ticketID, "", map[string]interface{}{
		"status": "open",
		"action": "created",
	}))
}
