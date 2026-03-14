package api

import (
	"net/http"
	"strconv"

	"be/internal/model"
	"be/internal/repo"
)

// handleStatus returns dashboard summary
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	ticketRepo := s.ticketRepo()

	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project is required")
		return
	}

	// Get pending tickets with blocked info
	pending, err := ticketRepo.GetPendingWithBlockedInfo(projectID, 20)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Get recently closed
	closed, err := ticketRepo.GetRecentlyClosed(projectID, 10)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Count by status
	allTickets, err := ticketRepo.List(&repo.ListFilter{ProjectID: projectID})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
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

	// Count ready (not blocked) and blocked
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

	statusLimit := r.URL.Query().Get("limit")
	limit := 10
	if statusLimit != "" {
		if l, err := strconv.Atoi(statusLimit); err == nil && l > 0 {
			limit = l
		}
	}

	// Trim pending to limit
	if len(pending) > limit {
		pending = pending[:limit]
	}

	if pending == nil {
		pending = []*repo.PendingTicket{}
	}
	if closed == nil {
		closed = []*model.Ticket{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"counts":          counts,
		"ready_count":     readyCount,
		"pending_tickets": pending,
		"recent_closed":   closed,
	})
}
