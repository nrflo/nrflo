package service

import (
	"encoding/json"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/proc"
	"be/internal/repo"
)

// AgentSessionLogService provides paginated agent session log queries.
type AgentSessionLogService struct {
	pool       *db.Pool
	clock      clock.Clock
	pidAlive   func(int64) bool
	pidMetrics func(int64) (int64, float64, int64, bool)
}

// NewAgentSessionLogService creates a new AgentSessionLogService.
func NewAgentSessionLogService(pool *db.Pool, clk clock.Clock) *AgentSessionLogService {
	return &AgentSessionLogService{
		pool:       pool,
		clock:      clk,
		pidAlive:   proc.PidAlive,
		pidMetrics: proc.PidMetrics,
	}
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

	findingRepo := repo.NewFindingRepo(s.pool, s.clock)
	result := make([]AgentSessionLogResponse, 0, len(rows))
	for _, row := range rows {
		resp := mapLogRow(row)
		// Fetch workflow_final_result from the findings table.
		if v, ok := findingRepo.GetSessionFindingByKey(row.WorkflowInstanceID, "workflow_final_result"); ok {
			var s string
			if json.Unmarshal(v, &s) == nil {
				resp.WorkflowFinalResult = s
			}
		}
		result = append(result, resp)
	}
	return result, total, nil
}

// LiveAgentSessionResponse is the JSON-ready shape for one live (running) session.
type LiveAgentSessionResponse struct {
	SessionID          string  `json:"session_id"`
	ProjectID          string  `json:"project_id"`
	AgentType          string  `json:"agent_type"`
	ModelID            string  `json:"model_id,omitempty"`
	WorkflowID         string  `json:"workflow_id"`
	WorkflowInstanceID string  `json:"workflow_instance_id"`
	Scheduled          bool    `json:"scheduled"`
	ExecutionMode      string  `json:"execution_mode,omitempty"`
	StartedAt          string  `json:"started_at,omitempty"`
	DurationSec        float64 `json:"duration_sec,omitempty"`
	PID                int64   `json:"pid"`
	RssKB              int64   `json:"rss_kb"`
	CpuPct             float64 `json:"cpu_pct"`
	OsUptimeSec        int64   `json:"os_uptime_sec"`
}

// ListLive returns currently running sessions for the given project, enriched with host metrics.
func (s *AgentSessionLogService) ListLive(projectID string) ([]LiveAgentSessionResponse, error) {
	r := repo.NewAgentSessionRepo(s.pool, s.clock)
	rows, err := r.ListLiveByProject(projectID)
	if err != nil {
		return nil, err
	}

	now := s.clock.Now()
	result := make([]LiveAgentSessionResponse, 0, len(rows))
	for _, row := range rows {
		pid := row.PID.Int64
		if !s.pidAlive(pid) {
			continue
		}
		rss, cpu, etime, _ := s.pidMetrics(pid)

		resp := LiveAgentSessionResponse{
			SessionID:          row.SessionID,
			ProjectID:          row.ProjectID,
			AgentType:          row.AgentType,
			WorkflowID:         row.WorkflowID,
			WorkflowInstanceID: row.WorkflowInstanceID,
			Scheduled:          row.ScheduledTaskID.Valid,
			PID:                pid,
			RssKB:              rss,
			CpuPct:             cpu,
			OsUptimeSec:        etime,
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
			started, err := time.Parse(time.RFC3339Nano, row.StartedAt.String)
			if err == nil {
				resp.DurationSec = now.Sub(started).Seconds()
			}
		}
		result = append(result, resp)
	}
	return result, nil
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

	return resp
}
