package repo

import (
	"encoding/json"
	"strings"

	"be/internal/model"
)

// WorkflowProgress holds completion data for a ticket's active workflow
type WorkflowProgress struct {
	WorkflowName    string `json:"workflow_name"`
	CurrentPhase    string `json:"current_phase"`
	CompletedPhases int    `json:"completed_phases"`
	TotalPhases     int    `json:"total_phases"`
	Status          string `json:"status"`
}

// PendingTicket is a ticket with blocked status info
type PendingTicket struct {
	*model.Ticket
	IsBlocked        bool              `json:"is_blocked"`
	BlockedBy        []string          `json:"blocked_by,omitempty"`
	WorkflowProgress *WorkflowProgress `json:"workflow_progress,omitempty"`
}

// MarshalJSON implements custom JSON marshaling for PendingTicket
func (pt PendingTicket) MarshalJSON() ([]byte, error) {
	// Get the ticket's marshaled form first
	ticketJSON, err := pt.Ticket.MarshalJSON()
	if err != nil {
		return nil, err
	}

	// Unmarshal into a map so we can add our fields
	var result map[string]interface{}
	if err := json.Unmarshal(ticketJSON, &result); err != nil {
		return nil, err
	}

	// Add the blocked info
	result["is_blocked"] = pt.IsBlocked
	if len(pt.BlockedBy) > 0 {
		result["blocked_by"] = pt.BlockedBy
	}
	if pt.WorkflowProgress != nil {
		result["workflow_progress"] = pt.WorkflowProgress
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
		IsBlocked        bool              `json:"is_blocked"`
		BlockedBy        []string          `json:"blocked_by,omitempty"`
		WorkflowProgress *WorkflowProgress `json:"workflow_progress,omitempty"`
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	pt.IsBlocked = aux.IsBlocked
	pt.BlockedBy = aux.BlockedBy
	pt.WorkflowProgress = aux.WorkflowProgress
	return nil
}

// ListWithBlockedInfo returns tickets with computed blocked status info
func (r *TicketRepo) ListWithBlockedInfo(filter *ListFilter) ([]*PendingTicket, error) {
	query := "SELECT " + ticketSelectColsPrefixed + " FROM tickets t WHERE LOWER(t.project_id) = LOWER(?)"
	args := []interface{}{filter.ProjectID}

	if filter.BlockedOnly {
		query += " AND t.status != 'closed' AND EXISTS ("
		query += "SELECT 1 FROM dependencies d "
		query += "INNER JOIN tickets blocker ON d.project_id = blocker.project_id AND d.depends_on_id = blocker.id "
		query += "WHERE d.project_id = t.project_id AND d.issue_id = t.id AND blocker.status != 'closed')"
	} else if filter.Status != "" {
		query += " AND t.status = ?"
		args = append(args, filter.Status)
	}

	if filter.IssueType != "" {
		query += " AND t.issue_type = ?"
		args = append(args, filter.IssueType)
	}

	query += " ORDER BY t.updated_at DESC, t.created_at DESC"

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tickets []*model.Ticket
	for rows.Next() {
		ticket, err := scanTicket(rows)
		if err != nil {
			return nil, err
		}
		tickets = append(tickets, ticket)
	}

	return r.attachBlockedInfo(tickets)
}

// GetPendingWithBlockedInfo returns non-closed tickets with their blocked status
func (r *TicketRepo) GetPendingWithBlockedInfo(projectID string, limit int) ([]*PendingTicket, error) {
	rows, err := r.db.Query(`
		SELECT `+ticketSelectColsPrefixed+`
		FROM tickets t
		WHERE LOWER(t.project_id) = LOWER(?) AND t.status != 'closed'
		ORDER BY t.priority ASC, t.created_at ASC
		LIMIT ?`, projectID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tickets []*model.Ticket
	for rows.Next() {
		ticket, err := scanTicket(rows)
		if err != nil {
			return nil, err
		}
		tickets = append(tickets, ticket)
	}

	return r.attachBlockedInfo(tickets)
}

func (r *TicketRepo) attachBlockedInfo(tickets []*model.Ticket) ([]*PendingTicket, error) {
	result := make([]*PendingTicket, 0, len(tickets))
	for _, ticket := range tickets {
		pt := &PendingTicket{Ticket: ticket}
		if ticket.Status != model.StatusClosed {
			blockers, err := r.getOpenBlockers(ticket.ProjectID, ticket.ID)
			if err != nil {
				return nil, err
			}
			pt.BlockedBy = blockers
			pt.IsBlocked = len(blockers) > 0
		}
		result = append(result, pt)
	}
	return result, nil
}

// getOpenBlockers returns IDs of open tickets that block the given ticket
func (r *TicketRepo) getOpenBlockers(projectID, ticketID string) ([]string, error) {
	rows, err := r.db.Query(`
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

// GetRecentlyClosed returns recently closed tickets
func (r *TicketRepo) GetRecentlyClosed(projectID string, limit int) ([]*model.Ticket, error) {
	rows, err := r.db.Query(`
		SELECT `+ticketSelectCols+`
		FROM tickets
		WHERE LOWER(project_id) = LOWER(?) AND status = 'closed'
		ORDER BY closed_at DESC
		LIMIT ?`, projectID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tickets []*model.Ticket
	for rows.Next() {
		ticket, err := scanTicket(rows)
		if err != nil {
			return nil, err
		}
		tickets = append(tickets, ticket)
	}

	return tickets, nil
}

// AttachWorkflowProgress enriches tickets with pre-computed workflow progress data.
func AttachWorkflowProgress(tickets []*PendingTicket, progress map[string]*WorkflowProgress) {
	for _, pt := range tickets {
		wp, ok := progress[strings.ToLower(pt.Ticket.ID)]
		if !ok {
			continue
		}
		pt.WorkflowProgress = wp
	}
}

// GetReady returns tickets that are not blocked by any open dependencies
func (r *TicketRepo) GetReady(projectID string) ([]*model.Ticket, error) {
	rows, err := r.db.Query(`
		SELECT `+ticketSelectColsPrefixed+`
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
		ticket, err := scanTicket(rows)
		if err != nil {
			return nil, err
		}
		tickets = append(tickets, ticket)
	}

	return tickets, nil
}
