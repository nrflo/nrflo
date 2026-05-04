package types

// WorkflowChainStepRequest defines one step in a workflow chain create/append request.
type WorkflowChainStepRequest struct {
	ID                   string `json:"id,omitempty"`
	WorkflowName         string `json:"workflow_name"`
	ScopeType            string `json:"scope_type"`
	BaseInstructions     string `json:"base_instructions,omitempty"`
	RequireTicketHandoff bool   `json:"require_ticket_handoff,omitempty"`
}

// WorkflowChainCreateRequest is the request body for creating a workflow chain.
type WorkflowChainCreateRequest struct {
	ID          string                     `json:"id,omitempty"`
	Name        string                     `json:"name"`
	Description string                     `json:"description,omitempty"`
	Steps       []WorkflowChainStepRequest `json:"steps"`
}

// WorkflowChainUpdateRequest is the request body for patching a workflow chain (partial update).
type WorkflowChainUpdateRequest struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
}

// WorkflowChainStepUpdateRequest is the request body for patching a single chain step.
type WorkflowChainStepUpdateRequest struct {
	WorkflowName         *string `json:"workflow_name,omitempty"`
	ScopeType            *string `json:"scope_type,omitempty"`
	BaseInstructions     *string `json:"base_instructions,omitempty"`
	RequireTicketHandoff *bool   `json:"require_ticket_handoff,omitempty"`
}

// ReorderStepsRequest specifies the desired step order by step IDs.
type ReorderStepsRequest struct {
	OrderedStepIDs []string `json:"ordered_step_ids"`
}
