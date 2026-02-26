package repo

import (
	"regexp"
	"strings"

	"be/internal/model"
)

var nonAlphanumRe = regexp.MustCompile(`[^a-zA-Z0-9]+`)

// sanitizeFTS5Query escapes user input for safe use in FTS5 MATCH.
// Splits on non-alphanumeric chars, wraps each token in double quotes.
func sanitizeFTS5Query(q string) string {
	tokens := nonAlphanumRe.Split(q, -1)
	var quoted []string
	for _, t := range tokens {
		if t == "" {
			continue
		}
		t = strings.ReplaceAll(t, `"`, `""`)
		quoted = append(quoted, `"`+t+`"`)
	}
	return strings.Join(quoted, " ")
}

// Search performs FTS5 search on tickets within a project
func (r *TicketRepo) Search(projectID, query string) ([]*model.Ticket, error) {
	sanitized := sanitizeFTS5Query(query)
	if sanitized == "" {
		return nil, nil
	}
	rows, err := r.db.Query(`
		SELECT `+ticketSelectColsPrefixed+`
		FROM tickets t
		INNER JOIN tickets_fts fts ON t.project_id = fts.project_id AND t.id = fts.id
		WHERE fts.project_id = ? AND tickets_fts MATCH ?
		ORDER BY rank`, strings.ToLower(projectID), sanitized)
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

// SearchWithBlockedInfo returns search results with computed blocked info
func (r *TicketRepo) SearchWithBlockedInfo(projectID, query string) ([]*PendingTicket, error) {
	tickets, err := r.Search(projectID, query)
	if err != nil {
		return nil, err
	}

	return r.attachBlockedInfo(tickets)
}
