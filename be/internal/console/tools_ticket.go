package console

import (
	"context"
	"encoding/json"

	"be/internal/model"
	"be/internal/repo"
	"be/internal/spawner/apirun"
	"be/internal/spawner/apirun/provider"
)

// ticketListHandler implements ticket_list: a filterable, compact picker over
// the console session's project. It reuses the same repo.ListWithBlockedInfo
// call GET /api/v1/tickets makes (blocked + pagination + sort), unlike the
// thinner TicketService.List (status/type only, no limit).
//
// Neither ticket tool takes a caller-supplied project id: both scope to
// env.ProjectID, the session's project fixed at session creation, and the repo
// filters on project_id. A console session can only run a workflow in its own
// project, so a cross-project ticket id would be unusable anyway — hence no
// project arg and no loadGuardedInstance-style guard.
type ticketListHandler struct{ d Deps }

func (ticketListHandler) Spec() provider.ToolSpec {
	return provider.ToolSpec{
		Name:        "ticket_list",
		Description: "List this project's tickets for selection. Filter by status (open/in_progress/closed/blocked) or issue type; returns a compact list (id, title, status, issue_type, priority, is_blocked). Use ticket_get for the full description, then pass a ticket's id to workflow_run as ticket_id.",
		InputSchema: json.RawMessage(`{
"type":"object",
"properties":{
"status":{"type":"string","description":"Filter: open, in_progress, closed, or blocked (open-with-open-blockers)"},
"type":{"type":"string","description":"Filter by issue type: bug, feature, task, or epic"},
"limit":{"type":"integer","description":"Max tickets to return (default 30, max 100)"}
},
"additionalProperties":false
}`),
	}
}

func (h ticketListHandler) Invoke(ctx context.Context, env apirun.ToolEnv, input json.RawMessage) (string, bool, error) {
	var args struct {
		Status string `json:"status"`
		Type   string `json:"type"`
		Limit  int    `json:"limit"`
	}
	if len(input) > 0 {
		if err := json.Unmarshal(input, &args); err != nil {
			return invalidArgs(err)
		}
	}
	if h.d.Pool == nil {
		return missingService("pool")
	}

	filter := &repo.ListFilter{
		ProjectID: env.ProjectID,
		IssueType: args.Type,
		Page:      1,
		PerPage:   30,
	}
	// "blocked" is a derived filter, not a stored status — mirror handlers_tickets.go.
	if args.Status == "blocked" {
		filter.BlockedOnly = true
	} else {
		filter.Status = args.Status
	}
	if args.Limit > 0 {
		if args.Limit > 100 {
			args.Limit = 100
		}
		filter.PerPage = args.Limit
	}

	result, err := repo.NewTicketRepo(h.d.Pool, h.d.Clock).ListWithBlockedInfo(filter)
	if err != nil {
		return err.Error(), true, nil
	}

	type item struct {
		ID        string          `json:"id"`
		Title     string          `json:"title"`
		Status    model.Status    `json:"status"`
		IssueType model.IssueType `json:"issue_type"`
		Priority  int             `json:"priority"`
		IsBlocked bool            `json:"is_blocked"`
	}
	out := make([]item, 0, len(result.Tickets))
	for _, t := range result.Tickets {
		out = append(out, item{
			ID:        t.ID,
			Title:     t.Title,
			Status:    t.Status,
			IssueType: t.IssueType,
			Priority:  t.Priority,
			IsBlocked: t.IsBlocked,
		})
	}
	b, err := json.Marshal(out)
	if err != nil {
		return err.Error(), true, nil
	}
	return string(b), false, nil
}

// ticketGetHandler implements ticket_get: the full ticket row (including the
// description) for one ticket in the session's project.
type ticketGetHandler struct{ d Deps }

func (ticketGetHandler) Spec() provider.ToolSpec {
	return provider.ToolSpec{
		Name:        "ticket_get",
		Description: "Get one ticket's full detail (including its description) by id, within this project.",
		InputSchema: json.RawMessage(`{
"type":"object",
"properties":{"ticket_id":{"type":"string"}},
"required":["ticket_id"],
"additionalProperties":false
}`),
	}
}

func (h ticketGetHandler) Invoke(ctx context.Context, env apirun.ToolEnv, input json.RawMessage) (string, bool, error) {
	var args struct {
		TicketID string `json:"ticket_id"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return invalidArgs(err)
	}
	if args.TicketID == "" {
		return "ticket_id is required", true, nil
	}
	if h.d.TicketSvc == nil {
		return missingService("ticket")
	}
	ticket, err := h.d.TicketSvc.Get(env.ProjectID, args.TicketID)
	if err != nil {
		return err.Error(), true, nil
	}
	b, err := json.Marshal(ticket)
	if err != nil {
		return err.Error(), true, nil
	}
	return string(b), false, nil
}

// ticketCurrentHandler implements ticket_current: the ticket the session is
// working on, detected from the caller's git branch at session open and stamped
// on the session row (agent_sessions.ticket_id). Returns the full ticket when
// set, or {"current_ticket":null} when the session has none (branch was not a
// known ticket, or the caller passed none) — never an error, so the model can
// branch on the null and fall back to ticket_list.
type ticketCurrentHandler struct{ d Deps }

func (ticketCurrentHandler) Spec() provider.ToolSpec {
	return provider.ToolSpec{
		Name:        "ticket_current",
		Description: "Return the ticket this session is working on (detected from the caller's git branch at launch), or null when there is none. Prefer its id as workflow_run's ticket_id unless the user names another; fall back to ticket_list when null.",
		InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false}`),
	}
}

func (h ticketCurrentHandler) Invoke(ctx context.Context, env apirun.ToolEnv, _ json.RawMessage) (string, bool, error) {
	if h.d.Pool == nil {
		return missingService("pool")
	}
	none := `{"current_ticket":null}`
	sess, err := repo.NewAgentSessionRepo(h.d.Pool, h.d.Clock).Get(env.SessionID)
	if err != nil || sess == nil || sess.TicketID == "" {
		return none, false, nil
	}
	if h.d.TicketSvc == nil {
		return missingService("ticket")
	}
	// Stamped id may have been closed/deleted since launch — degrade to the bare id.
	ticket, gerr := h.d.TicketSvc.Get(env.ProjectID, sess.TicketID)
	if gerr != nil {
		b, _ := json.Marshal(map[string]string{"current_ticket": sess.TicketID})
		return string(b), false, nil
	}
	b, err := json.Marshal(map[string]interface{}{"current_ticket": ticket})
	if err != nil {
		return err.Error(), true, nil
	}
	return string(b), false, nil
}
