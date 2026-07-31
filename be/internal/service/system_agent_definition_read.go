package service

import (
	"database/sql"
	"fmt"
	"time"

	"be/internal/model"
)

// Get retrieves a single system agent definition by id
func (s *SystemAgentDefinitionService) Get(id string) (*model.SystemAgentDefinition, error) {
	def := &model.SystemAgentDefinition{}
	var createdAt, updatedAt string
	var restartThreshold, maxFailRestarts, stallStartTimeout, stallRunningTimeout, apiMaxIterations, apiMaxTokens sql.NullInt64
	var reasoningEffort sql.NullString
	var tier sql.NullInt64

	err := s.pool.QueryRow(`
		SELECT id, role, model, timeout, prompt, tools, api_max_iterations, api_max_tokens,
		       restart_threshold, max_fail_restarts, stall_start_timeout_sec, stall_running_timeout_sec,
		       execution_mode, reasoning_effort, tier, isolate_worktree, created_at, updated_at
		FROM system_agent_definitions
		WHERE LOWER(id) = LOWER(?)`, id).Scan(
		&def.ID, &def.Role, &def.Model, &def.Timeout, &def.Prompt, &def.Tools, &apiMaxIterations, &apiMaxTokens,
		&restartThreshold, &maxFailRestarts, &stallStartTimeout, &stallRunningTimeout,
		&def.ExecutionMode, &reasoningEffort, &tier, &def.IsolateWorktree, &createdAt, &updatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("system agent definition not found: %s", id)
	}
	if err != nil {
		return nil, err
	}

	def.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	def.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	scanNullableInts(def, restartThreshold, maxFailRestarts, stallStartTimeout, stallRunningTimeout, apiMaxIterations, apiMaxTokens)
	if reasoningEffort.Valid {
		v := reasoningEffort.String
		def.ReasoningEffort = &v
	}
	if tier.Valid {
		v := int(tier.Int64)
		def.Tier = &v
	}
	return def, nil
}

// GetForBackend retrieves a system agent definition by role and execution_mode.
// Returns sql.ErrNoRows unwrapped if no match so callers can choose a fallback.
func (s *SystemAgentDefinitionService) GetForBackend(role, backend string) (*model.SystemAgentDefinition, error) {
	def := &model.SystemAgentDefinition{}
	var createdAt, updatedAt string
	var restartThreshold, maxFailRestarts, stallStartTimeout, stallRunningTimeout, apiMaxIterations, apiMaxTokens sql.NullInt64
	var reasoningEffort sql.NullString
	var tier sql.NullInt64

	err := s.pool.QueryRow(`
		SELECT id, role, model, timeout, prompt, tools, api_max_iterations, api_max_tokens,
		       restart_threshold, max_fail_restarts, stall_start_timeout_sec, stall_running_timeout_sec,
		       execution_mode, reasoning_effort, tier, isolate_worktree, created_at, updated_at
		FROM system_agent_definitions
		WHERE role = ? AND execution_mode = ?
		LIMIT 1`, role, backend).Scan(
		&def.ID, &def.Role, &def.Model, &def.Timeout, &def.Prompt, &def.Tools, &apiMaxIterations, &apiMaxTokens,
		&restartThreshold, &maxFailRestarts, &stallStartTimeout, &stallRunningTimeout,
		&def.ExecutionMode, &reasoningEffort, &tier, &def.IsolateWorktree, &createdAt, &updatedAt,
	)
	if err != nil {
		return nil, err // sql.ErrNoRows returned unwrapped for caller fallback
	}

	def.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	def.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	scanNullableInts(def, restartThreshold, maxFailRestarts, stallStartTimeout, stallRunningTimeout, apiMaxIterations, apiMaxTokens)
	if reasoningEffort.Valid {
		v := reasoningEffort.String
		def.ReasoningEffort = &v
	}
	if tier.Valid {
		v := int(tier.Int64)
		def.Tier = &v
	}
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
	q := `SELECT id, role, model, timeout, prompt, tools, api_max_iterations, api_max_tokens,
		       restart_threshold, max_fail_restarts, stall_start_timeout_sec, stall_running_timeout_sec,
		       execution_mode, reasoning_effort, tier, isolate_worktree, created_at, updated_at
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
		var restartThreshold, maxFailRestarts, stallStartTimeout, stallRunningTimeout, apiMaxIterations, apiMaxTokens sql.NullInt64
		var reasoningEffort sql.NullString
		var tier sql.NullInt64

		err := rows.Scan(
			&def.ID, &def.Role, &def.Model, &def.Timeout, &def.Prompt, &def.Tools, &apiMaxIterations, &apiMaxTokens,
			&restartThreshold, &maxFailRestarts, &stallStartTimeout, &stallRunningTimeout,
			&def.ExecutionMode, &reasoningEffort, &tier, &def.IsolateWorktree, &createdAt, &updatedAt,
		)
		if err != nil {
			return nil, err
		}

		def.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		def.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
		scanNullableInts(def, restartThreshold, maxFailRestarts, stallStartTimeout, stallRunningTimeout, apiMaxIterations, apiMaxTokens)
		if reasoningEffort.Valid {
			v := reasoningEffort.String
			def.ReasoningEffort = &v
		}
		if tier.Valid {
			v := int(tier.Int64)
			def.Tier = &v
		}
		defs = append(defs, def)
	}

	return defs, nil
}

// scanNullableInts populates nullable int pointer fields on the model from sql.NullInt64 scan vars.
func scanNullableInts(def *model.SystemAgentDefinition, restart, maxFail, stallStart, stallRunning, apiMax, apiMaxTokens sql.NullInt64) {
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
	if apiMaxTokens.Valid {
		v := int(apiMaxTokens.Int64)
		def.APIMaxTokens = &v
	}
}
