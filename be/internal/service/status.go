package service

import (
	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"
)

// StatusService builds the dashboard summary shared by GET /api/v1/status and
// the console project_status tool.
type StatusService struct {
	pool  *db.Pool
	clock clock.Clock
}

// NewStatusService creates a new StatusService.
func NewStatusService(pool *db.Pool, clk clock.Clock) *StatusService {
	return &StatusService{pool: pool, clock: clk}
}

// ProjectStatus returns pending tickets (trimmed to limit), recently closed
// tickets, and per-status/blocked/ready counts for projectID. limit<=0 falls
// back to 10, matching the original handler default.
func (s *StatusService) ProjectStatus(projectID string, limit int) (map[string]interface{}, error) {
	ticketRepo := repo.NewTicketRepo(s.pool, s.clock)

	pending, err := ticketRepo.GetPendingWithBlockedInfo(projectID, 20)
	if err != nil {
		return nil, err
	}

	closed, err := ticketRepo.GetRecentlyClosed(projectID, 10)
	if err != nil {
		return nil, err
	}

	allTickets, err := ticketRepo.List(&repo.ListFilter{ProjectID: projectID})
	if err != nil {
		return nil, err
	}

	counts := map[string]int{
		"open":        0,
		"in_progress": 0,
		"closed":      0,
		"blocked":     0,
		"total":       len(allTickets),
	}
	for _, t := range allTickets {
		counts[string(t.Status)]++
	}

	readyCount := 0
	blockedCount := 0
	for _, p := range pending {
		if p.IsBlocked {
			blockedCount++
			continue
		}
		readyCount++
	}
	counts["blocked"] = blockedCount

	if limit <= 0 {
		limit = 10
	}
	if len(pending) > limit {
		pending = pending[:limit]
	}

	if pending == nil {
		pending = []*repo.PendingTicket{}
	}
	if closed == nil {
		closed = []*model.Ticket{}
	}

	return map[string]interface{}{
		"counts":          counts,
		"ready_count":     readyCount,
		"pending_tickets": pending,
		"recent_closed":   closed,
	}, nil
}
