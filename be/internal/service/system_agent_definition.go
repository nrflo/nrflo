package service

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/types"
)

// SystemAgentDefinitionService handles system agent definition business logic
type SystemAgentDefinitionService struct {
	clock clock.Clock
	pool  *db.Pool
}

// NewSystemAgentDefinitionService creates a new system agent definition service
func NewSystemAgentDefinitionService(pool *db.Pool, clk clock.Clock) *SystemAgentDefinitionService {
	return &SystemAgentDefinitionService{pool: pool, clock: clk}
}

// Create creates a new system agent definition
func (s *SystemAgentDefinitionService) Create(req *types.SystemAgentDefCreateRequest) (*model.SystemAgentDefinition, error) {
	if req.ID == "" {
		return nil, fmt.Errorf("agent id is required")
	}

	modelName := req.Model
	if modelName == "" {
		modelName = "sonnet"
	}
	timeout := req.Timeout
	if timeout == 0 {
		timeout = 20
	}

	now := s.clock.Now().UTC().Format(time.RFC3339Nano)
	id := strings.ToLower(req.ID)

	_, err := s.pool.Exec(`
		INSERT INTO system_agent_definitions (id, model, timeout, prompt, restart_threshold, max_fail_restarts, stall_start_timeout_sec, stall_running_timeout_sec, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, modelName, timeout, req.Prompt, req.RestartThreshold, req.MaxFailRestarts, req.StallStartTimeoutSec, req.StallRunningTimeoutSec, now, now,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") || strings.Contains(err.Error(), "already exists") {
			return nil, fmt.Errorf("system agent definition already exists: %s", req.ID)
		}
		return nil, err
	}

	ts, _ := time.Parse(time.RFC3339Nano, now)
	return &model.SystemAgentDefinition{
		ID:                     id,
		Model:                  modelName,
		Timeout:                timeout,
		Prompt:                 req.Prompt,
		RestartThreshold:       req.RestartThreshold,
		MaxFailRestarts:        req.MaxFailRestarts,
		StallStartTimeoutSec:   req.StallStartTimeoutSec,
		StallRunningTimeoutSec: req.StallRunningTimeoutSec,
		CreatedAt:              ts,
		UpdatedAt:              ts,
	}, nil
}

// Get retrieves a single system agent definition
func (s *SystemAgentDefinitionService) Get(id string) (*model.SystemAgentDefinition, error) {
	def := &model.SystemAgentDefinition{}
	var createdAt, updatedAt string
	var restartThreshold, maxFailRestarts, stallStartTimeout, stallRunningTimeout sql.NullInt64

	err := s.pool.QueryRow(`
		SELECT id, model, timeout, prompt, restart_threshold, max_fail_restarts, stall_start_timeout_sec, stall_running_timeout_sec, created_at, updated_at
		FROM system_agent_definitions
		WHERE LOWER(id) = LOWER(?)`, id).Scan(
		&def.ID, &def.Model, &def.Timeout, &def.Prompt,
		&restartThreshold, &maxFailRestarts, &stallStartTimeout, &stallRunningTimeout,
		&createdAt, &updatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("system agent definition not found: %s", id)
	}
	if err != nil {
		return nil, err
	}

	def.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	def.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	if restartThreshold.Valid {
		v := int(restartThreshold.Int64)
		def.RestartThreshold = &v
	}
	if maxFailRestarts.Valid {
		v := int(maxFailRestarts.Int64)
		def.MaxFailRestarts = &v
	}
	if stallStartTimeout.Valid {
		v := int(stallStartTimeout.Int64)
		def.StallStartTimeoutSec = &v
	}
	if stallRunningTimeout.Valid {
		v := int(stallRunningTimeout.Int64)
		def.StallRunningTimeoutSec = &v
	}
	return def, nil
}

// List retrieves all system agent definitions
func (s *SystemAgentDefinitionService) List() ([]*model.SystemAgentDefinition, error) {
	rows, err := s.pool.Query(`
		SELECT id, model, timeout, prompt, restart_threshold, max_fail_restarts, stall_start_timeout_sec, stall_running_timeout_sec, created_at, updated_at
		FROM system_agent_definitions
		ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	defs := []*model.SystemAgentDefinition{}
	for rows.Next() {
		def := &model.SystemAgentDefinition{}
		var createdAt, updatedAt string
		var restartThreshold, maxFailRestarts, stallStartTimeout, stallRunningTimeout sql.NullInt64

		err := rows.Scan(
			&def.ID, &def.Model, &def.Timeout, &def.Prompt,
			&restartThreshold, &maxFailRestarts, &stallStartTimeout, &stallRunningTimeout,
			&createdAt, &updatedAt,
		)
		if err != nil {
			return nil, err
		}

		def.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		def.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
		if restartThreshold.Valid {
			v := int(restartThreshold.Int64)
			def.RestartThreshold = &v
		}
		if maxFailRestarts.Valid {
			v := int(maxFailRestarts.Int64)
			def.MaxFailRestarts = &v
		}
		if stallStartTimeout.Valid {
			v := int(stallStartTimeout.Int64)
			def.StallStartTimeoutSec = &v
		}
		if stallRunningTimeout.Valid {
			v := int(stallRunningTimeout.Int64)
			def.StallRunningTimeoutSec = &v
		}
		defs = append(defs, def)
	}

	return defs, nil
}

// Update updates a system agent definition
func (s *SystemAgentDefinitionService) Update(id string, req *types.SystemAgentDefUpdateRequest) error {
	updates := []string{}
	args := []interface{}{}

	if req.Model != nil {
		updates = append(updates, "model = ?")
		args = append(args, *req.Model)
	}
	if req.Timeout != nil {
		updates = append(updates, "timeout = ?")
		args = append(args, *req.Timeout)
	}
	if req.Prompt != nil {
		updates = append(updates, "prompt = ?")
		args = append(args, *req.Prompt)
	}
	if req.RestartThreshold != nil {
		updates = append(updates, "restart_threshold = ?")
		args = append(args, *req.RestartThreshold)
	}
	if req.MaxFailRestarts != nil {
		updates = append(updates, "max_fail_restarts = ?")
		args = append(args, *req.MaxFailRestarts)
	}
	if req.StallStartTimeoutSec != nil {
		updates = append(updates, "stall_start_timeout_sec = ?")
		args = append(args, *req.StallStartTimeoutSec)
	}
	if req.StallRunningTimeoutSec != nil {
		updates = append(updates, "stall_running_timeout_sec = ?")
		args = append(args, *req.StallRunningTimeoutSec)
	}

	if len(updates) == 0 {
		return nil
	}

	now := s.clock.Now().UTC().Format(time.RFC3339Nano)
	updates = append(updates, "updated_at = ?")
	args = append(args, now)
	args = append(args, id)

	query := "UPDATE system_agent_definitions SET " + strings.Join(updates, ", ") +
		" WHERE LOWER(id) = LOWER(?)"

	result, err := s.pool.Exec(query, args...)
	if err != nil {
		return err
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("system agent definition not found: %s", id)
	}
	return nil
}

// Delete deletes a system agent definition
func (s *SystemAgentDefinitionService) Delete(id string) error {
	result, err := s.pool.Exec(
		"DELETE FROM system_agent_definitions WHERE LOWER(id) = LOWER(?)", id)
	if err != nil {
		return err
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("system agent definition not found: %s", id)
	}
	return nil
}
