package service

import (
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
	clock    clock.Clock
	pool     *db.Pool
	modelSvc *ModelService
}

// NewSystemAgentDefinitionService creates a new system agent definition service
func NewSystemAgentDefinitionService(pool *db.Pool, clk clock.Clock, modelSvc *ModelService) *SystemAgentDefinitionService {
	return &SystemAgentDefinitionService{pool: pool, clock: clk, modelSvc: modelSvc}
}

// Create creates a new system agent definition
func (s *SystemAgentDefinitionService) Create(req *types.SystemAgentDefCreateRequest) (*model.SystemAgentDefinition, error) {
	if req.ID == "" {
		return nil, validationErrorf("agent id is required")
	}

	modelName := req.Model
	timeout := req.Timeout
	if timeout == 0 {
		timeout = 20
	}

	executionMode := req.ExecutionMode
	if executionMode == "" {
		executionMode = "cli_interactive"
	} else if err := validateExecutionMode(executionMode); err != nil {
		return nil, err
	}

	if modelName == "" {
		if req.Tier == nil {
			return nil, validationErrorf("model or tier is required")
		}
	} else if s.modelSvc != nil {
		valid, err := s.modelSvc.IsValidModelForMode(modelName, registryMode(executionMode))
		if err != nil {
			return nil, fmt.Errorf("failed to validate model: %w", err)
		}
		if !valid {
			return nil, validationErrorf("invalid model: %q", modelName)
		}
	}

	role := req.Role
	id := strings.ToLower(req.ID)
	if role == "" {
		role = id
	}
	if role == "planner" && !csvGrantsTool(req.Tools, "emit_findings") {
		return nil, validationErrorf("planner agent requires the emit_findings tool in its tools CSV")
	}

	if err := validateDefReasoningEffort(s.modelSvc, executionMode, modelName, req.ReasoningEffort); err != nil {
		return nil, err
	}

	now := s.clock.Now().UTC().Format(time.RFC3339Nano)

	_, err := s.pool.Exec(`
		INSERT INTO system_agent_definitions
			(id, role, model, timeout, prompt, tools, api_max_iterations, api_max_tokens,
			 restart_threshold, max_fail_restarts, stall_start_timeout_sec, stall_running_timeout_sec,
			 execution_mode, reasoning_effort, tier, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, role, modelName, timeout, req.Prompt, req.Tools, req.APIMaxIterations, req.APIMaxTokens,
		req.RestartThreshold, req.MaxFailRestarts, req.StallStartTimeoutSec, req.StallRunningTimeoutSec,
		executionMode, req.ReasoningEffort, req.Tier, now, now,
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
		APIMaxTokens:           req.APIMaxTokens,
		RestartThreshold:       req.RestartThreshold,
		MaxFailRestarts:        req.MaxFailRestarts,
		StallStartTimeoutSec:   req.StallStartTimeoutSec,
		StallRunningTimeoutSec: req.StallRunningTimeoutSec,
		ReasoningEffort:        req.ReasoningEffort,
		Tier:                   req.Tier,
		CreatedAt:              ts,
		UpdatedAt:              ts,
	}, nil
}

// Update updates a system agent definition
func (s *SystemAgentDefinitionService) Update(id string, req *types.SystemAgentDefUpdateRequest) error {
	if err := s.revalidatePlannerTools(id, req); err != nil {
		return err
	}
	if err := s.revalidateModel(id, req); err != nil {
		return err
	}
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
		if *req.Model != "" && s.modelSvc != nil {
			// Determine effective execution_mode to branch validation
			var currentMode string
			if req.ExecutionMode != nil {
				currentMode = *req.ExecutionMode
			} else {
				if scanErr := s.pool.QueryRow(
					"SELECT execution_mode FROM system_agent_definitions WHERE LOWER(id) = LOWER(?)",
					id).Scan(&currentMode); scanErr != nil {
					return fmt.Errorf("failed to load system agent definition: %w", scanErr)
				}
			}
			if currentMode != "script" {
				valid, vErr := s.modelSvc.IsValidModelForMode(*req.Model, registryMode(currentMode))
				if vErr != nil {
					return fmt.Errorf("failed to validate model: %w", vErr)
				}
				if !valid {
					return validationErrorf("invalid model: %q", *req.Model)
				}
			}
		}
		updates = append(updates, "model = ?")
		args = append(args, *req.Model)
	}
	if req.Tier != nil {
		updates = append(updates, "tier = ?")
		args = append(args, *req.Tier)
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
	if req.APIMaxTokens != nil {
		updates = append(updates, "api_max_tokens = ?")
		args = append(args, *req.APIMaxTokens)
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
	if req.ReasoningEffort != nil {
		mode := req.ExecutionMode
		modelName := req.Model
		if mode == nil || modelName == nil {
			var currentMode, currentModel string
			if scanErr := s.pool.QueryRow(
				"SELECT execution_mode, model FROM system_agent_definitions WHERE LOWER(id) = LOWER(?)",
				id).Scan(&currentMode, &currentModel); scanErr != nil {
				return fmt.Errorf("failed to load system agent definition: %w", scanErr)
			}
			if mode == nil {
				mode = &currentMode
			}
			if modelName == nil {
				modelName = &currentModel
			}
		}
		if err := validateDefReasoningEffort(s.modelSvc, *mode, *modelName, req.ReasoningEffort); err != nil {
			return err
		}
		updates = append(updates, "reasoning_effort = ?")
		args = append(args, *req.ReasoningEffort)
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
