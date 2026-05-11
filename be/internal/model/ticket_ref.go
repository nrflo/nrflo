package model

import (
	"database/sql"
	"time"
)

// TicketRefKind is the type of external reference linked to a ticket.
type TicketRefKind string

const (
	KindSource    TicketRefKind = "source"
	KindRelated   TicketRefKind = "related"
	KindPR        TicketRefKind = "pr"
	KindDesignDoc TicketRefKind = "design_doc"
)

// ValidKinds returns all valid TicketRefKind values.
func ValidKinds() []TicketRefKind {
	return []TicketRefKind{KindSource, KindRelated, KindPR, KindDesignDoc}
}

// IsValidKind reports whether s is a known TicketRefKind.
func IsValidKind(s string) bool {
	for _, k := range ValidKinds() {
		if string(k) == s {
			return true
		}
	}
	return false
}

// TicketRef is an external link (PR, design doc, etc.) associated with a ticket.
type TicketRef struct {
	ID        int64          `json:"id"`
	ProjectID string         `json:"project_id"`
	TicketID  string         `json:"ticket_id"`
	Kind      string         `json:"kind"`
	URL       string         `json:"url"`
	Label     sql.NullString `json:"label,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
}
