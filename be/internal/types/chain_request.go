package types

// ChainCreateRequest is the request for creating a chain execution
type ChainCreateRequest struct {
	Name         string   `json:"name"`
	WorkflowName string   `json:"workflow_name"`
	Category     string   `json:"category,omitempty"`
	TicketIDs    []string `json:"ticket_ids"`
}

// ChainUpdateRequest is the request for updating a pending chain execution
type ChainUpdateRequest struct {
	Name      *string  `json:"name,omitempty"`
	TicketIDs []string `json:"ticket_ids,omitempty"`
}
