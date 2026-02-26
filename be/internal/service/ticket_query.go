package service

import (
	"encoding/json"

	"be/internal/model"
)

// PendingTicket is a ticket with blocked status info
type PendingTicket struct {
	*model.Ticket
	IsBlocked bool     `json:"is_blocked"`
	BlockedBy []string `json:"blocked_by,omitempty"`
}

// MarshalJSON implements custom JSON marshaling for PendingTicket
func (pt PendingTicket) MarshalJSON() ([]byte, error) {
	ticketJSON, err := pt.Ticket.MarshalJSON()
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(ticketJSON, &result); err != nil {
		return nil, err
	}

	result["is_blocked"] = pt.IsBlocked
	if len(pt.BlockedBy) > 0 {
		result["blocked_by"] = pt.BlockedBy
	}

	return json.Marshal(result)
}

// UnmarshalJSON implements custom JSON unmarshaling for PendingTicket.
// Required because *model.Ticket.UnmarshalJSON would be promoted and called
// on a nil embedded pointer, causing a panic.
func (pt *PendingTicket) UnmarshalJSON(data []byte) error {
	if pt.Ticket == nil {
		pt.Ticket = &model.Ticket{}
	}
	if err := pt.Ticket.UnmarshalJSON(data); err != nil {
		return err
	}
	var aux struct {
		IsBlocked bool     `json:"is_blocked"`
		BlockedBy []string `json:"blocked_by,omitempty"`
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	pt.IsBlocked = aux.IsBlocked
	pt.BlockedBy = aux.BlockedBy
	return nil
}

// GetReady returns tickets that are not blocked
func (s *TicketService) GetReady(projectID string) ([]*model.Ticket, error) {
	rows, err := s.pool.Query(`
		SELECT `+ticketColsT+`
		FROM tickets t
		WHERE LOWER(t.project_id) = LOWER(?) AND t.status != 'closed'
		AND NOT EXISTS (
			SELECT 1 FROM dependencies d
			INNER JOIN tickets blocker ON d.project_id = blocker.project_id AND d.depends_on_id = blocker.id
			WHERE d.project_id = t.project_id AND d.issue_id = t.id AND blocker.status != 'closed'
		)
		ORDER BY t.priority ASC, t.created_at ASC`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tickets []*model.Ticket
	for rows.Next() {
		ticket, err := scanTicketRow(rows)
		if err != nil {
			return nil, err
		}
		tickets = append(tickets, ticket)
	}

	return tickets, nil
}

// GetStatus returns ticket status summary
func (s *TicketService) GetStatus(projectID string, pendingLimit, completedLimit int) (map[string]interface{}, error) {
	// Get pending tickets
	rows, err := s.pool.Query(`
		SELECT `+ticketColsT+`
		FROM tickets t
		WHERE LOWER(t.project_id) = LOWER(?) AND t.status != 'closed'
		ORDER BY t.priority ASC, t.created_at ASC
		LIMIT ?`, projectID, pendingLimit)
	if err != nil {
		return nil, err
	}

	var pending []*PendingTicket
	for rows.Next() {
		ticket, err := scanTicketRow(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		pending = append(pending, &PendingTicket{Ticket: ticket})
	}
	rows.Close()

	// Get blockers for each pending ticket
	for _, pt := range pending {
		blockers, err := s.getOpenBlockers(pt.ProjectID, pt.ID)
		if err != nil {
			return nil, err
		}
		pt.BlockedBy = blockers
		pt.IsBlocked = len(blockers) > 0
	}

	// Get completed tickets
	rows, err = s.pool.Query(`
		SELECT `+ticketCols+`
		FROM tickets
		WHERE LOWER(project_id) = LOWER(?) AND status = 'closed'
		ORDER BY closed_at DESC
		LIMIT ?`, projectID, completedLimit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var completed []*model.Ticket
	for rows.Next() {
		ticket, err := scanTicketRow(rows)
		if err != nil {
			return nil, err
		}
		completed = append(completed, ticket)
	}

	return map[string]interface{}{
		"pending":   pending,
		"completed": completed,
	}, nil
}

func (s *TicketService) getOpenBlockers(projectID, ticketID string) ([]string, error) {
	rows, err := s.pool.Query(`
		SELECT blocker.id
		FROM dependencies d
		INNER JOIN tickets blocker ON d.project_id = blocker.project_id AND d.depends_on_id = blocker.id
		WHERE LOWER(d.project_id) = LOWER(?) AND LOWER(d.issue_id) = LOWER(?) AND blocker.status != 'closed'`, projectID, ticketID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var blockers []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		blockers = append(blockers, id)
	}
	return blockers, nil
}
