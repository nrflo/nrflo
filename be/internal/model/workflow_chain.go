package model

import (
	"database/sql"
	"time"
)

// WorkflowChain is a named sequence of workflow steps within a project.
type WorkflowChain struct {
	ID          string    `json:"id"`
	ProjectID   string    `json:"project_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// WorkflowChainStep defines one step in a WorkflowChain.
type WorkflowChainStep struct {
	ID                   string    `json:"id"`
	ProjectID            string    `json:"project_id"`
	ChainID              string    `json:"chain_id"`
	Position             int       `json:"position"`
	WorkflowName         string    `json:"workflow_name"`
	ScopeType            string    `json:"scope_type"`
	BaseInstructions     string    `json:"base_instructions"`
	RequireTicketHandoff bool      `json:"require_ticket_handoff"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

// WorkflowChainRun is a single execution of a WorkflowChain.
type WorkflowChainRun struct {
	ID                  string     `json:"id"`
	ProjectID           string     `json:"project_id"`
	ChainID             string     `json:"chain_id"`
	Status              string     `json:"status"`
	InitialInstructions string     `json:"initial_instructions"`
	TriggeredBy         string     `json:"triggered_by"`
	CurrentPosition     int        `json:"current_position"`
	StartedAt           *time.Time `json:"started_at,omitempty"`
	CompletedAt         *time.Time `json:"completed_at,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

// WorkflowChainRunStep is the materialized snapshot of one step within a WorkflowChainRun.
type WorkflowChainRunStep struct {
	ID                   string         `json:"id"`
	ChainRunID           string         `json:"chain_run_id"`
	Position             int            `json:"position"`
	WorkflowName         string         `json:"workflow_name"`
	ScopeType            string         `json:"scope_type"`
	RequireTicketHandoff bool           `json:"require_ticket_handoff"`
	WorkflowInstanceID   sql.NullString `json:"-"`
	TicketID             sql.NullString `json:"-"`
	InstructionsUsed     string         `json:"instructions_used"`
	Status               string         `json:"status"`
	StartedAt            sql.NullString `json:"-"`
	EndedAt              sql.NullString `json:"-"`
	CreatedAt            time.Time      `json:"created_at"`
	UpdatedAt            time.Time      `json:"updated_at"`
}
