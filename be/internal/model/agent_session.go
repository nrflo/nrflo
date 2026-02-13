package model

import (
	"database/sql"
	"encoding/json"
	"time"
)

// AgentSessionStatus represents the status of an agent session
type AgentSessionStatus string

const (
	AgentSessionRunning          AgentSessionStatus = "running"
	AgentSessionCompleted        AgentSessionStatus = "completed"
	AgentSessionFailed           AgentSessionStatus = "failed"
	AgentSessionTimeout          AgentSessionStatus = "timeout"
	AgentSessionContinued        AgentSessionStatus = "continued"
	AgentSessionProjectCompleted AgentSessionStatus = "project_completed"
	AgentSessionCallback         AgentSessionStatus = "callback"
)

// AgentSession represents a spawned agent session
type AgentSession struct {
	ID                 string             `json:"id"`
	ProjectID          string             `json:"project_id"`
	TicketID           string             `json:"ticket_id"`
	WorkflowInstanceID string             `json:"workflow_instance_id"`
	Phase              string             `json:"phase"`
	AgentType          string             `json:"agent_type"`
	ModelID            sql.NullString     `json:"-"`
	Status             AgentSessionStatus `json:"status"`
	Result             sql.NullString     `json:"-"` // pass | fail | continue | timeout | callback
	ResultReason       sql.NullString     `json:"-"`
	PID                sql.NullInt64      `json:"-"`
	Findings           sql.NullString     `json:"-"` // JSON findings
	ContextLeft        sql.NullInt64      `json:"-"` // Remaining context percentage
	AncestorSessionID  sql.NullString     `json:"-"` // Ancestor session for continuation
	SpawnCommand       sql.NullString     `json:"-"` // Full CLI command used to spawn
	PromptContext      sql.NullString     `json:"-"` // System prompt file contents
	RestartCount       int                `json:"-"` // Number of low-context restarts
	StartedAt          sql.NullString     `json:"-"`
	EndedAt            sql.NullString     `json:"-"`
	CreatedAt          time.Time          `json:"created_at"`
	UpdatedAt          time.Time          `json:"updated_at"`

	// Derived fields (populated via JOIN, not stored in agent_sessions)
	Workflow string `json:"-"` // workflow_id from workflow_instances (for API compat)

	// Populated from agent_messages table (not stored in agent_sessions)
	Messages     []string `json:"-"` // Full messages from agent_messages table
	MessageCount int      `json:"-"` // Total message count from agent_messages table
}

// GetFindings returns the session findings as a map
func (as *AgentSession) GetFindings() map[string]interface{} {
	if !as.Findings.Valid || as.Findings.String == "" {
		return make(map[string]interface{})
	}
	findings := make(map[string]interface{})
	json.Unmarshal([]byte(as.Findings.String), &findings)
	return findings
}

// SetFindings updates the findings JSON from a map
func (as *AgentSession) SetFindings(findings map[string]interface{}) {
	data, _ := json.Marshal(findings)
	as.Findings = sql.NullString{String: string(data), Valid: true}
}

// MarshalJSON implements custom JSON marshaling for AgentSession
func (as AgentSession) MarshalJSON() ([]byte, error) {
	messages := as.Messages
	if messages == nil {
		messages = []string{}
	}

	var modelID *string
	if as.ModelID.Valid {
		modelID = &as.ModelID.String
	}
	var spawnCommand *string
	if as.SpawnCommand.Valid {
		spawnCommand = &as.SpawnCommand.String
	}
	var promptContext *string
	if as.PromptContext.Valid {
		promptContext = &as.PromptContext.String
	}
	var contextLeft *int
	if as.ContextLeft.Valid {
		v := int(as.ContextLeft.Int64)
		contextLeft = &v
	}
	var ancestorSessionID *string
	if as.AncestorSessionID.Valid {
		ancestorSessionID = &as.AncestorSessionID.String
	}
	var result *string
	if as.Result.Valid {
		result = &as.Result.String
	}
	var resultReason *string
	if as.ResultReason.Valid {
		resultReason = &as.ResultReason.String
	}
	var pid *int
	if as.PID.Valid {
		v := int(as.PID.Int64)
		pid = &v
	}
	var findings interface{}
	if as.Findings.Valid && as.Findings.String != "" {
		json.Unmarshal([]byte(as.Findings.String), &findings)
	}
	var startedAt *string
	if as.StartedAt.Valid {
		startedAt = &as.StartedAt.String
	}
	var endedAt *string
	if as.EndedAt.Valid {
		endedAt = &as.EndedAt.String
	}

	// Use Workflow derived field for backward compatibility
	workflow := as.Workflow

	return json.Marshal(&struct {
		ID                 string             `json:"id"`
		ProjectID          string             `json:"project_id"`
		TicketID           string             `json:"ticket_id"`
		WorkflowInstanceID string             `json:"workflow_instance_id"`
		Phase              string             `json:"phase"`
		Workflow           string             `json:"workflow"`
		AgentType          string             `json:"agent_type"`
		ModelID            *string            `json:"model_id,omitempty"`
		Status             AgentSessionStatus `json:"status"`
		Result             *string            `json:"result,omitempty"`
		ResultReason       *string            `json:"result_reason,omitempty"`
		PID                *int               `json:"pid,omitempty"`
		Findings           interface{}        `json:"findings,omitempty"`
		LastMessages       []string           `json:"last_messages"`
		MessageCount       int                `json:"message_count"`
		ContextLeft        *int               `json:"context_left,omitempty"`
		RestartCount       int                `json:"restart_count"`
		AncestorSessionID  *string            `json:"ancestor_session_id,omitempty"`
		SpawnCommand       *string            `json:"spawn_command,omitempty"`
		PromptContext      *string            `json:"prompt_context,omitempty"`
		StartedAt          *string            `json:"started_at,omitempty"`
		EndedAt            *string            `json:"ended_at,omitempty"`
		CreatedAt          time.Time          `json:"created_at"`
		UpdatedAt          time.Time          `json:"updated_at"`
	}{
		ID:                 as.ID,
		ProjectID:          as.ProjectID,
		TicketID:           as.TicketID,
		WorkflowInstanceID: as.WorkflowInstanceID,
		Phase:              as.Phase,
		Workflow:           workflow,
		AgentType:          as.AgentType,
		ModelID:            modelID,
		Status:             as.Status,
		Result:             result,
		ResultReason:       resultReason,
		PID:                pid,
		Findings:           findings,
		LastMessages:       messages,
		MessageCount:       as.MessageCount,
		ContextLeft:        contextLeft,
		RestartCount:       as.RestartCount,
		AncestorSessionID:  ancestorSessionID,
		SpawnCommand:       spawnCommand,
		PromptContext:      promptContext,
		StartedAt:          startedAt,
		EndedAt:            endedAt,
		CreatedAt:          as.CreatedAt,
		UpdatedAt:          as.UpdatedAt,
	})
}
