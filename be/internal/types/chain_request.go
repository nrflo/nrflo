package types

// ChainCreateRequest is the request for creating a chain execution
type ChainCreateRequest struct {
	Name             string   `json:"name"`
	WorkflowName     string   `json:"workflow_name"`
	EpicTicketID     string   `json:"epic_ticket_id,omitempty"`
	TicketIDs        []string `json:"ticket_ids"`
	OrderedTicketIDs []string `json:"ordered_ticket_ids,omitempty"`
}

// ChainUpdateRequest is the request for updating a pending chain execution
type ChainUpdateRequest struct {
	Name             *string  `json:"name,omitempty"`
	TicketIDs        []string `json:"ticket_ids,omitempty"`
	OrderedTicketIDs []string `json:"ordered_ticket_ids,omitempty"`
}

// ChainAppendRequest is the request for appending tickets to a running chain
type ChainAppendRequest struct {
	TicketIDs []string `json:"ticket_ids"`
}

// ChainRemoveRequest is the request for removing pending tickets from a running chain
type ChainRemoveRequest struct {
	TicketIDs []string `json:"ticket_ids"`
}

// ChainPreviewRequest is the request for previewing a chain's expanded tickets and dependencies
type ChainPreviewRequest struct {
	TicketIDs []string `json:"ticket_ids"`
}

// ChainPreviewResponse is the response from a chain preview
type ChainPreviewResponse struct {
	TicketIDs   []string            `json:"ticket_ids"`
	Deps        map[string][]string `json:"deps"`
	AddedByDeps []string            `json:"added_by_deps"`
}
