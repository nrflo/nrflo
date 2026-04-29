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

func validateExecutionMode(mode string) error {
	if mode != "cli" && mode != "api" {
		return fmt.Errorf("invalid execution_mode: must be 'cli' or 'api'")
	}
	return nil
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

	executionMode := req.ExecutionMode
	if executionMode == "" {
		executionMode = "cli"
	} else if err := validateExecutionMode(executionMode); err != nil {
		return nil, err
	}

	role := req.Role
	id := strings.ToLower(req.ID)
	if role == "" {
		role = id
	}

	now := s.clock.Now().UTC().Format(time.RFC3339Nano)

	_, err := s.pool.Exec(`
		INSERT INTO system_agent_definitions
			(id, role, model, timeout, prompt, tools, api_max_iterations,
			 restart_threshold, max_fail_restarts, stall_start_timeout_sec, stall_running_timeout_sec,
			 execution_mode, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, role, modelName, timeout, req.Prompt, req.Tools, req.APIMaxIterations,
		req.RestartThreshold, req.MaxFailRestarts, req.StallStartTimeoutSec, req.StallRunningTimeoutSec,
		executionMode, now, now,
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
		Role:                   role,
		ExecutionMode:          executionMode,
		Model:                  modelName,
		Timeout:                timeout,
		Prompt:                 req.Prompt,
		Tools:                  req.Tools,
		APIMaxIterations:       req.APIMaxIterations,
		RestartThreshold:       req.RestartThreshold,
		MaxFailRestarts:        req.MaxFailRestarts,
		StallStartTimeoutSec:   req.StallStartTimeoutSec,
		StallRunningTimeoutSec: req.StallRunningTimeoutSec,
		CreatedAt:              ts,
		UpdatedAt:              ts,
	}, nil
}

// Get retrieves a single system agent definition by id
func (s *SystemAgentDefinitionService) Get(id string) (*model.SystemAgentDefinition, error) {
	def := &model.SystemAgentDefinition{}
	var createdAt, updatedAt string
	var restartThreshold, maxFailRestarts, stallStartTimeout, stallRunningTimeout, apiMaxIterations sql.NullInt64

	err := s.pool.QueryRow(`
		SELECT id, role, model, timeout, prompt, tools, api_max_iterations,
		       restart_threshold, max_fail_restarts, stall_start_timeout_sec, stall_running_timeout_sec,
		       execution_mode, created_at, updated_at
		FROM system_agent_definitions
		WHERE LOWER(id) = LOWER(?)`, id).Scan(
		&def.ID, &def.Role, &def.Model, &def.Timeout, &def.Prompt, &def.Tools, &apiMaxIterations,
		&restartThreshold, &maxFailRestarts, &stallStartTimeout, &stallRunningTimeout,
		&def.ExecutionMode, &createdAt, &updatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("system agent definition not found: %s", id)
	}
	if err != nil {
		return nil, err
	}

	def.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	def.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	scanNullableInts(def, restartThreshold, maxFailRestarts, stallStartTimeout, stallRunningTimeout, apiMaxIterations)
	return def, nil
}

// GetForBackend retrieves a system agent definition by role and execution_mode.
// Returns sql.ErrNoRows unwrapped if no match so callers can choose a fallback.
func (s *SystemAgentDefinitionService) GetForBackend(role, backend string) (*model.SystemAgentDefinition, error) {
	def := &model.SystemAgentDefinition{}
	var createdAt, updatedAt string
	var restartThreshold, maxFailRestarts, stallStartTimeout, stallRunningTimeout, apiMaxIterations sql.NullInt64

	err := s.pool.QueryRow(`
		SELECT id, role, model, timeout, prompt, tools, api_max_iterations,
		       restart_threshold, max_fail_restarts, stall_start_timeout_sec, stall_running_timeout_sec,
		       execution_mode, created_at, updated_at
		FROM system_agent_definitions
		WHERE role = ? AND execution_mode = ?
		LIMIT 1`, role, backend).Scan(
		&def.ID, &def.Role, &def.Model, &def.Timeout, &def.Prompt, &def.Tools, &apiMaxIterations,
		&restartThreshold, &maxFailRestarts, &stallStartTimeout, &stallRunningTimeout,
		&def.ExecutionMode, &createdAt, &updatedAt,
	)
	if err != nil {
		return nil, err // sql.ErrNoRows returned unwrapped for caller fallback
	}

	def.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	def.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	scanNullableInts(def, restartThreshold, maxFailRestarts, stallStartTimeout, stallRunningTimeout, apiMaxIterations)
	return def, nil
}

// List retrieves all system agent definitions.
func (s *SystemAgentDefinitionService) List() ([]*model.SystemAgentDefinition, error) {
	return s.listQuery("")
}

// ListForAPI retrieves system agent definitions for the HTTP list endpoint.
// When includeAPIMode is false, execution_mode='api' rows are excluded so they
// remain hidden in cli-mode servers while still being resolvable by GetForBackend.
func (s *SystemAgentDefinitionService) ListForAPI(includeAPIMode bool) ([]*model.SystemAgentDefinition, error) {
	filter := ""
	if !includeAPIMode {
		filter = "WHERE execution_mode <> 'api'"
	}
	return s.listQuery(filter)
}

func (s *SystemAgentDefinitionService) listQuery(whereClause string) ([]*model.SystemAgentDefinition, error) {
	q := `SELECT id, role, model, timeout, prompt, tools, api_max_iterations,
		       restart_threshold, max_fail_restarts, stall_start_timeout_sec, stall_running_timeout_sec,
		       execution_mode, created_at, updated_at
		FROM system_agent_definitions`
	if whereClause != "" {
		q += " " + whereClause
	}
	q += " ORDER BY id"

	rows, err := s.pool.Query(q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	defs := []*model.SystemAgentDefinition{}
	for rows.Next() {
		def := &model.SystemAgentDefinition{}
		var createdAt, updatedAt string
		var restartThreshold, maxFailRestarts, stallStartTimeout, stallRunningTimeout, apiMaxIterations sql.NullInt64

		err := rows.Scan(
			&def.ID, &def.Role, &def.Model, &def.Timeout, &def.Prompt, &def.Tools, &apiMaxIterations,
			&restartThreshold, &maxFailRestarts, &stallStartTimeout, &stallRunningTimeout,
			&def.ExecutionMode, &createdAt, &updatedAt,
		)
		if err != nil {
			return nil, err
		}

		def.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		def.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
		scanNullableInts(def, restartThreshold, maxFailRestarts, stallStartTimeout, stallRunningTimeout, apiMaxIterations)
		defs = append(defs, def)
	}

	return defs, nil
}

// Update updates a system agent definition
func (s *SystemAgentDefinitionService) Update(id string, req *types.SystemAgentDefUpdateRequest) error {
	updates := []string{}
	args := []interface{}{}

	if req.Role != nil {
		updates = append(updates, "role = ?")
		args = append(args, *req.Role)
	}
	if req.ExecutionMode != nil {
		if err := validateExecutionMode(*req.ExecutionMode); err != nil {
			return err
		}
		updates = append(updates, "execution_mode = ?")
		args = append(args, *req.ExecutionMode)
	}
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
	if req.Tools != nil {
		updates = append(updates, "tools = ?")
		args = append(args, *req.Tools)
	}
	if req.APIMaxIterations != nil {
		updates = append(updates, "api_max_iterations = ?")
		args = append(args, *req.APIMaxIterations)
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

// scanNullableInts populates nullable int pointer fields on the model from sql.NullInt64 scan vars.
func scanNullableInts(def *model.SystemAgentDefinition, restart, maxFail, stallStart, stallRunning, apiMax sql.NullInt64) {
	if restart.Valid {
		v := int(restart.Int64)
		def.RestartThreshold = &v
	}
	if maxFail.Valid {
		v := int(maxFail.Int64)
		def.MaxFailRestarts = &v
	}
	if stallStart.Valid {
		v := int(stallStart.Int64)
		def.StallStartTimeoutSec = &v
	}
	if stallRunning.Valid {
		v := int(stallRunning.Int64)
		def.StallRunningTimeoutSec = &v
	}
	if apiMax.Valid {
		v := int(apiMax.Int64)
		def.APIMaxIterations = &v
	}
}
