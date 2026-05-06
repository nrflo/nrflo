package service

import (
	"encoding/json"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"
)

// AgentSessionLogService provides paginated agent session log queries.
type AgentSessionLogService struct {
	pool  *db.Pool
	clock clock.Clock
}

// NewAgentSessionLogService creates a new AgentSessionLogService.
func NewAgentSessionLogService(pool *db.Pool, clk clock.Clock) *AgentSessionLogService {
	return &AgentSessionLogService{pool: pool, clock: clk}
}

// AgentSessionLogResponse is the JSON-ready shape for one log row.
type AgentSessionLogResponse struct {
	SessionID           string  `json:"session_id"`
	ProjectID           string  `json:"project_id"`
	AgentType           string  `json:"agent_type"`
	ModelID             string  `json:"model_id,omitempty"`
	Status              string  `json:"status"`
	StartedAt           string  `json:"started_at,omitempty"`
	EndedAt             string  `json:"ended_at,omitempty"`
	DurationSec         float64 `json:"duration_sec,omitempty"`
	WorkflowID          string  `json:"workflow_id"`
	WorkflowInstanceID  string  `json:"workflow_instance_id"`
	Scheduled           bool    `json:"scheduled"`
	ExecutionMode       string  `json:"execution_mode,omitempty"`
	WorkflowFinalResult string  `json:"workflow_final_result,omitempty"`
}

// List returns a paginated list of finished agent sessions for the given project.
func (s *AgentSessionLogService) List(projectID string, page, perPage int) ([]AgentSessionLogResponse, int, error) {
	r := repo.NewAgentSessionRepo(s.pool, s.clock)
	rows, total, err := r.ListFinished(repo.ListFinishedFilter{ProjectID: projectID}, page, perPage)
	if err != nil {
		return nil, 0, err
	}

	result := make([]AgentSessionLogResponse, 0, len(rows))
	for _, row := range rows {
		resp := mapLogRow(row)
		result = append(result, resp)
	}
	return result, total, nil
}

func mapLogRow(row *model.AgentSessionLogRow) AgentSessionLogResponse {
	resp := AgentSessionLogResponse{
		SessionID:          row.SessionID,
		ProjectID:          row.ProjectID,
		AgentType:          row.AgentType,
		Status:             string(row.Status),
		WorkflowID:         row.WorkflowID,
		WorkflowInstanceID: row.WorkflowInstanceID,
		Scheduled:          row.ScheduledTaskID.Valid,
	}

	if row.ModelID.Valid {
		resp.ModelID = row.ModelID.String
	}
	if row.EffectiveMode.Valid && row.EffectiveMode.String != "" {
		resp.ExecutionMode = row.EffectiveMode.String
	} else if row.ExecutionMode.Valid {
		resp.ExecutionMode = row.ExecutionMode.String
	}
	if row.StartedAt.Valid {
		resp.StartedAt = row.StartedAt.String
	}
	if row.EndedAt.Valid {
		resp.EndedAt = row.EndedAt.String
	}

	if row.StartedAt.Valid && row.EndedAt.Valid {
		started, err1 := time.Parse(time.RFC3339Nano, row.StartedAt.String)
		ended, err2 := time.Parse(time.RFC3339Nano, row.EndedAt.String)
		if err1 == nil && err2 == nil && ended.After(started) {
			resp.DurationSec = ended.Sub(started).Seconds()
		}
	}

	if row.Findings.Valid && row.Findings.String != "" {
		resp.WorkflowFinalResult = extractWorkflowFinalResult(row.Findings.String)
	}

	return resp
}

func extractWorkflowFinalResult(findingsJSON string) string {
	var findings map[string]interface{}
	if json.Unmarshal([]byte(findingsJSON), &findings) != nil {
		return ""
	}
	val, ok := findings["workflow_final_result"]
	if !ok {
		return ""
	}
	s, ok := val.(string)
	if !ok {
		return ""
	}
	return s
}
