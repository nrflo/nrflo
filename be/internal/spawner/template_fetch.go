package spawner

import (
	"context"
	"encoding/json"

	"be/internal/logger"
	"be/internal/repo"
)

// fetchTicketInfo returns the ticket title and description for template expansion.
func (s *Spawner) fetchTicketInfo(projectID, ticketID string) (title, description string) {
	pool := s.pool()
	if pool == nil {
		logger.Warn(context.Background(), "no database pool for ticket info")
		return ticketID, "_No description available_"
	}

	ticketRepo := repo.NewTicketRepo(pool, s.config.Clock)
	ticket, err := ticketRepo.Get(projectID, ticketID)
	if err != nil {
		logger.Warn(context.Background(), "failed to fetch ticket", "ticket_id", ticketID, "error", err)
		return ticketID, "_No description available_"
	}
	title = ticket.Title
	if ticket.Description.Valid && ticket.Description.String != "" {
		description = ticket.Description.String
	} else {
		description = "_No description available_"
	}
	return title, description
}

// fetchUserInstructionsRaw returns user_instructions from the workflow instance findings.
// Returns "" on miss. Uses wfiID directly when available; falls back to ticket-based lookup.
func (s *Spawner) fetchUserInstructionsRaw(projectID, ticketID, workflowName, wfiID string) string {
	pool := s.pool()
	if pool == nil {
		logger.Warn(context.Background(), "no database pool for user instructions")
		return ""
	}

	resolvedWFIID := s.resolveWFIID(projectID, ticketID, workflowName, wfiID)
	if resolvedWFIID == "" {
		return ""
	}

	findingRepo := repo.NewFindingRepo(pool, s.config.Clock)
	raw, err := findingRepo.GetOwn("workflow_instance", resolvedWFIID)
	if err != nil {
		return ""
	}
	if v, ok := raw["user_instructions"]; ok {
		var str string
		if json.Unmarshal(v, &str) == nil {
			return str
		}
	}
	return ""
}

// fetchCallbackRaw returns raw callback instructions and from_agent from workflow instance findings.
// Returns ("", "") on miss. Uses wfiID directly when available; falls back to ticket-based lookup.
func (s *Spawner) fetchCallbackRaw(projectID, ticketID, workflowName, wfiID string) (instructions string, fromAgent string) {
	pool := s.pool()
	if pool == nil {
		logger.Warn(context.Background(), "no database pool for callback instructions")
		return "", ""
	}

	resolvedWFIID := s.resolveWFIID(projectID, ticketID, workflowName, wfiID)
	if resolvedWFIID == "" {
		return "", ""
	}

	findingRepo := repo.NewFindingRepo(pool, s.config.Clock)
	raw, err := findingRepo.GetOwn("workflow_instance", resolvedWFIID)
	if err != nil {
		return "", ""
	}
	cbRaw, ok := raw["_callback"]
	if !ok {
		return "", ""
	}
	var callbackMap map[string]interface{}
	if json.Unmarshal(cbRaw, &callbackMap) != nil {
		return "", ""
	}
	instr, _ := callbackMap["instructions"].(string)
	if instr == "" {
		return "", ""
	}
	from, _ := callbackMap["from_agent"].(string)
	return instr, from
}
