package model

import (
	"database/sql"
	"encoding/json"
	"time"
)

// AgentSessionStatus represents the status of an agent session
type AgentSessionStatus string

const (
	AgentSessionRunning   AgentSessionStatus = "running"
	AgentSessionCompleted AgentSessionStatus = "completed"
	AgentSessionFailed    AgentSessionStatus = "failed"
	AgentSessionTimeout   AgentSessionStatus = "timeout"
)

// AgentSession represents a spawned agent session
type AgentSession struct {
	ID            string             `json:"id"`
	ProjectID     string             `json:"project_id"`
	TicketID      string             `json:"ticket_id"`
	Phase         string             `json:"phase"`
	Workflow      string             `json:"workflow"`
	AgentType     string             `json:"agent_type"`
	ModelID       sql.NullString     `json:"-"`
	Status        AgentSessionStatus `json:"status"`
	LastMessages  sql.NullString     `json:"-"` // JSON array of strings
	MessageStats  sql.NullString     `json:"-"` // JSON object: {"tool:Read": 5, "skill:commit -m \"msg\"": 1}
	SpawnCommand  sql.NullString     `json:"-"` // Full CLI command used to spawn
	PromptContext sql.NullString     `json:"-"` // System prompt file contents
	CreatedAt     time.Time          `json:"created_at"`
	UpdatedAt     time.Time          `json:"updated_at"`
}

// MarshalJSON implements custom JSON marshaling for AgentSession
func (as AgentSession) MarshalJSON() ([]byte, error) {
	// Parse last_messages JSON into []string
	var messages []string
	if as.LastMessages.Valid && as.LastMessages.String != "" {
		_ = json.Unmarshal([]byte(as.LastMessages.String), &messages)
	}
	if messages == nil {
		messages = []string{}
	}

	// Parse message_stats JSON into map[string]int
	var stats map[string]int
	if as.MessageStats.Valid && as.MessageStats.String != "" {
		_ = json.Unmarshal([]byte(as.MessageStats.String), &stats)
	}
	if stats == nil {
		stats = map[string]int{}
	}

	// Handle optional model_id
	var modelID *string
	if as.ModelID.Valid {
		modelID = &as.ModelID.String
	}

	// Handle optional spawn_command
	var spawnCommand *string
	if as.SpawnCommand.Valid {
		spawnCommand = &as.SpawnCommand.String
	}

	// Handle optional prompt_context
	var promptContext *string
	if as.PromptContext.Valid {
		promptContext = &as.PromptContext.String
	}

	return json.Marshal(&struct {
		ID            string             `json:"id"`
		ProjectID     string             `json:"project_id"`
		TicketID      string             `json:"ticket_id"`
		Phase         string             `json:"phase"`
		Workflow      string             `json:"workflow"`
		AgentType     string             `json:"agent_type"`
		ModelID       *string            `json:"model_id,omitempty"`
		Status        AgentSessionStatus `json:"status"`
		LastMessages  []string           `json:"last_messages"`
		MessageStats  map[string]int     `json:"message_stats"`
		SpawnCommand  *string            `json:"spawn_command,omitempty"`
		PromptContext *string            `json:"prompt_context,omitempty"`
		CreatedAt     time.Time          `json:"created_at"`
		UpdatedAt     time.Time          `json:"updated_at"`
	}{
		ID:            as.ID,
		ProjectID:     as.ProjectID,
		TicketID:      as.TicketID,
		Phase:         as.Phase,
		Workflow:      as.Workflow,
		AgentType:     as.AgentType,
		ModelID:       modelID,
		Status:        as.Status,
		LastMessages:  messages,
		MessageStats:  stats,
		SpawnCommand:  spawnCommand,
		PromptContext: promptContext,
		CreatedAt:     as.CreatedAt,
		UpdatedAt:     as.UpdatedAt,
	})
}
