package types

// ChainCreateRequest is the request for creating a chain execution
type ChainCreateRequest struct {
	Name         string   `json:"name"`
	WorkflowName string   `json:"workflow_name"`
	EpicTicketID string   `json:"epic_ticket_id,omitempty"`
	TicketIDs    []string `json:"ticket_ids"`
}

// ChainUpdateRequest is the request for updating a pending chain execution
type ChainUpdateRequest struct {
	Name      *string  `json:"name,omitempty"`
	TicketIDs []string `json:"ticket_ids,omitempty"`
}

// ChainAppendRequest is the request for appending tickets to a running chain
type ChainAppendRequest struct {
	TicketIDs []string `json:"ticket_ids"`
}
