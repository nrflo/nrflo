package service

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/types"
)

// ErrAPIModeDisabled is returned when execution_mode="api" is used but api_mode_enabled is not set.
var ErrAPIModeDisabled = errors.New("api mode disabled")

// AgentDefinitionService handles agent definition business logic
type AgentDefinitionService struct {
	clock            clock.Clock
	pool             *db.Pool
	cliModelSvc      *CLIModelService
	apiModelSvc      *APIModelService
	pythonScriptRepo *repo.PythonScriptRepo
}

// NewAgentDefinitionService creates a new agent definition service
func NewAgentDefinitionService(pool *db.Pool, clk clock.Clock, cliModelSvc *CLIModelService, apiModelSvc *APIModelService, pythonScriptRepo *repo.PythonScriptRepo) *AgentDefinitionService {
	return &AgentDefinitionService{pool: pool, clock: clk, cliModelSvc: cliModelSvc, apiModelSvc: apiModelSvc, pythonScriptRepo: pythonScriptRepo}
}

// CreateAgentDef creates a new agent definition
func (s *AgentDefinitionService) CreateAgentDef(projectID, workflowID string, req *types.AgentDefCreateRequest) (*model.AgentDefinition, error) {
	if req.ID == "" {
		return nil, fmt.Errorf("agent id is required")
	}

	// Determine execution mode early so we can skip prompt requirement for scripts.
	executionMode := req.ExecutionMode
	if executionMode == "" {
		executionMode = "cli_interactive"
	}
	if executionMode != "cli_interactive" && executionMode != "api" && executionMode != "script" {
		return nil, fmt.Errorf("invalid execution_mode: %q", executionMode)
	}
	if executionMode == "api" {
		settingsSvc := NewGlobalSettingsService(s.pool, s.clock)
		apiModeVal, _ := settingsSvc.Get("api_mode_enabled")
		if apiModeVal != "true" {
			return nil, ErrAPIModeDisabled
		}
	}

	if req.Consultant && executionMode != "api" {
		return nil, fmt.Errorf("consultant agent requires execution_mode=api")
	}

	nodeRole, err0 := validateNodeRole(req.NodeRole, req.Consultant, req.Tools)
	if err0 != nil {
		return nil, err0
	}
	if err := validateDescription(nodeRole, req.Description); err != nil {
		return nil, err
	}

	if executionMode != "script" && req.Prompt == "" {
		return nil, fmt.Errorf("prompt is required")
	}

	// Script mode: enforce coupling rules.
	if executionMode == "script" {
		if err := s.validateScriptMode(projectID, req.PythonScriptID, req.Prompt, req.Tools, req.APIMaxIterations, req.APIMaxTokens); err != nil {
			return nil, err
		}
	} else if req.PythonScriptID != nil {
		return nil, fmt.Errorf("python_script_id_requires_script_mode")
	}

	// Verify workflow exists and get groups for tag validation
	var groupsStr string
	err := s.pool.QueryRow(
		"SELECT groups FROM workflows WHERE LOWER(project_id) = LOWER(?) AND LOWER(id) = LOWER(?)",
		projectID, workflowID).Scan(&groupsStr)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("workflow not found: %s", workflowID)
	}
	if err != nil {
		return nil, err
	}

	// Validate tag against workflow groups
	if req.Tag != "" {
		if err := validateTagInGroups(req.Tag, groupsStr); err != nil {
			return nil, err
		}
	}

	lcModel := strings.ToLower(req.LowConsumptionModel)
	if lcModel != "" && executionMode != "script" {
		switch executionMode {
		case "api":
			valid, err := s.apiModelSvc.IsValidModel(lcModel)
			if err != nil {
				return nil, fmt.Errorf("failed to validate low_consumption_model: %w", err)
			}
			if !valid {
				return nil, fmt.Errorf("invalid low_consumption_model: %q", lcModel)
			}
		default:
			valid, err := s.cliModelSvc.IsValidModel(lcModel)
			if err != nil {
				return nil, fmt.Errorf("failed to validate low_consumption_model: %w", err)
			}
			if !valid {
				return nil, fmt.Errorf("invalid low_consumption_model: %q", lcModel)
			}
		}
	}

	// Validate and marshal validation_commands
	validationCommandsJSON := "[]"
	if req.ValidationCommands != nil {
		if err := validateValidationCommands(*req.ValidationCommands); err != nil {
			return nil, err
		}
		b, err := json.Marshal(*req.ValidationCommands)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal validation_commands: %w", err)
		}
		validationCommandsJSON = string(b)
	}

	// Validate layer config (layer >= 0) with existing agents + new agent
	if err := s.validateLayerConfigForWorkflow(projectID, workflowID, req.ID, req.Layer); err != nil {
		return nil, err
	}

	// Defaults
	modelName := req.Model
	if executionMode == "script" {
		modelName = "script" // force sentinel model for script agents
	} else if modelName == "" {
		modelName = "sonnet"
	}

	// Validate model against api_models for api execution mode
	if executionMode == "api" {
		valid, err := s.apiModelSvc.IsValidModel(modelName)
		if err != nil {
			return nil, fmt.Errorf("failed to validate model: %w", err)
		}
		if !valid {
			return nil, fmt.Errorf("invalid model: %q", modelName)
		}
	}

	if err := validateDefReasoningEffort(s.cliModelSvc, s.apiModelSvc, executionMode, modelName, req.ReasoningEffort); err != nil {
		return nil, err
	}

	timeout := req.Timeout
	if timeout == 0 {
		timeout = 20
	}

	// Default stall_start_timeout to 0 (disabled) for script agents when not specified.
	stallStartTimeout := req.StallStartTimeoutSec
	if executionMode == "script" && stallStartTimeout == nil {
		zero := 0
		stallStartTimeout = &zero
	}

	now := s.clock.Now().UTC().Format(time.RFC3339Nano)
	id := strings.ToLower(req.ID)
	pid := strings.ToLower(projectID)
	wid := strings.ToLower(workflowID)

	_, err = s.pool.Exec(`
		INSERT INTO agent_definitions (id, project_id, workflow_id, model, timeout, prompt, restart_threshold, max_fail_restarts, stall_start_timeout_sec, stall_running_timeout_sec, tag, low_consumption_model, layer, execution_mode, tools, api_max_iterations, api_max_tokens, python_script_id, validation_commands, consultant, node_role, description, reasoning_effort, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, pid, wid, modelName, timeout, req.Prompt, req.RestartThreshold, req.MaxFailRestarts, stallStartTimeout, req.StallRunningTimeoutSec, req.Tag, lcModel, req.Layer, executionMode, req.Tools, req.APIMaxIterations, req.APIMaxTokens, req.PythonScriptID, validationCommandsJSON, req.Consultant, nodeRole, req.Description, req.ReasoningEffort, now, now,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") || strings.Contains(err.Error(), "already exists") {
			return nil, fmt.Errorf("agent definition already exists: %s", req.ID)
		}
		return nil, err
	}

	ts, _ := time.Parse(time.RFC3339Nano, now)
	return &model.AgentDefinition{
		ID:                     id,
		ProjectID:              pid,
		WorkflowID:             wid,
		Model:                  modelName,
		Timeout:                timeout,
		Prompt:                 req.Prompt,
		RestartThreshold:       req.RestartThreshold,
		MaxFailRestarts:        req.MaxFailRestarts,
		StallStartTimeoutSec:   stallStartTimeout,
		StallRunningTimeoutSec: req.StallRunningTimeoutSec,
		Tag:                    req.Tag,
		LowConsumptionModel:    lcModel,
		Layer:                  req.Layer,
		ExecutionMode:          executionMode,
		Tools:                  req.Tools,
		APIMaxIterations:       req.APIMaxIterations,
		APIMaxTokens:           req.APIMaxTokens,
		PythonScriptID:         req.PythonScriptID,
		ValidationCommands:     validationCommandsJSON,
		ReasoningEffort:        req.ReasoningEffort,
		Consultant:             req.Consultant,
		NodeRole:               nodeRole,
		Description:            req.Description,
		CreatedAt:              ts,
		UpdatedAt:              ts,
	}, nil
}

// UpdateAgentDef updates an agent definition
func (s *AgentDefinitionService) UpdateAgentDef(projectID, workflowID, id string, req *types.AgentDefUpdateRequest) error {
	updates := []string{}
	args := []interface{}{}

	// effectiveMode resolves the execution mode that will apply after the update.
	// Loaded lazily from DB when req.ExecutionMode is not set.
	var loadedMode string
	effectiveMode := func() (string, error) {
		if req.ExecutionMode != nil {
			return *req.ExecutionMode, nil
		}
		if loadedMode != "" {
			return loadedMode, nil
		}
		if err := s.pool.QueryRow(
			"SELECT execution_mode FROM agent_definitions WHERE LOWER(project_id) = LOWER(?) AND LOWER(workflow_id) = LOWER(?) AND LOWER(id) = LOWER(?)",
			projectID, workflowID, id).Scan(&loadedMode); err != nil {
			return "", fmt.Errorf("failed to load agent definition: %w", err)
		}
		return loadedMode, nil
	}

	if req.Model != nil {
		mode, err := effectiveMode()
		if err != nil {
			return err
		}
		if mode == "api" {
			valid, vErr := s.apiModelSvc.IsValidModel(*req.Model)
			if vErr != nil {
				return fmt.Errorf("failed to validate model: %w", vErr)
			}
			if !valid {
				return fmt.Errorf("invalid model: %q", *req.Model)
			}
		}
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
	if req.Layer != nil {
		// Validate layer config (layer >= 0) with updated layer value
		if err := s.validateLayerConfigForWorkflow(projectID, workflowID, id, *req.Layer); err != nil {
			return err
		}
		// If layer changes, ensure the old layer's policy remains valid
		var oldLayer int
		if scanErr := s.pool.QueryRow(
			"SELECT layer FROM agent_definitions WHERE LOWER(project_id) = LOWER(?) AND LOWER(workflow_id) = LOWER(?) AND LOWER(id) = LOWER(?)",
			projectID, workflowID, id).Scan(&oldLayer); scanErr == nil && oldLayer != *req.Layer {
			var remaining int
			s.pool.QueryRow(
				`SELECT COUNT(*) FROM agent_definitions
				 WHERE LOWER(project_id) = LOWER(?) AND LOWER(workflow_id) = LOWER(?)
				   AND layer = ? AND LOWER(id) != LOWER(?) AND consultant = 0 AND node_role = 'static'`,
				projectID, workflowID, oldLayer, id).Scan(&remaining)
			if err := s.validatePolicyNotViolatedByLayerChange(projectID, workflowID, oldLayer, remaining); err != nil {
				return err
			}
		}
		updates = append(updates, "layer = ?")
		args = append(args, *req.Layer)
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
	if req.Tag != nil {
		if *req.Tag != "" {
			// Validate tag against workflow groups
			var groupsStr string
			err := s.pool.QueryRow(
				"SELECT groups FROM workflows WHERE LOWER(project_id) = LOWER(?) AND LOWER(id) = LOWER(?)",
				projectID, workflowID).Scan(&groupsStr)
			if err != nil {
				return fmt.Errorf("failed to load workflow for tag validation: %w", err)
			}
			if err := validateTagInGroups(*req.Tag, groupsStr); err != nil {
				return err
			}
		}
		updates = append(updates, "tag = ?")
		args = append(args, *req.Tag)
	}
	if req.LowConsumptionModel != nil {
		lcModel := strings.ToLower(*req.LowConsumptionModel)
		if lcModel != "" {
			mode, err := effectiveMode()
			if err != nil {
				return err
			}
			switch mode {
			case "api":
				valid, vErr := s.apiModelSvc.IsValidModel(lcModel)
				if vErr != nil {
					return fmt.Errorf("failed to validate low_consumption_model: %w", vErr)
				}
				if !valid {
					return fmt.Errorf("invalid low_consumption_model: %q", lcModel)
				}
			case "script":
				// skip validation for script mode
			default:
				valid, vErr := s.cliModelSvc.IsValidModel(lcModel)
				if vErr != nil {
					return fmt.Errorf("failed to validate low_consumption_model: %w", vErr)
				}
				if !valid {
					return fmt.Errorf("invalid low_consumption_model: %q", lcModel)
				}
			}
		}
		updates = append(updates, "low_consumption_model = ?")
		args = append(args, lcModel)
	}
	if req.ExecutionMode != nil {
		mode := *req.ExecutionMode
		if mode != "cli_interactive" && mode != "api" && mode != "script" {
			return fmt.Errorf("invalid execution_mode: %q", mode)
		}
		if mode == "api" {
			settingsSvc := NewGlobalSettingsService(s.pool, s.clock)
			apiModeVal, _ := settingsSvc.Get("api_mode_enabled")
			if apiModeVal != "true" {
				return ErrAPIModeDisabled
			}
		}
		if mode == "script" {
			// When switching to script mode, validate script coupling rules.
			prompt := ""
			if req.Prompt != nil {
				prompt = *req.Prompt
			}
			tools := ""
			if req.Tools != nil {
				tools = *req.Tools
			}
			if err := s.validateScriptMode(projectID, req.PythonScriptID, prompt, tools, req.APIMaxIterations, req.APIMaxTokens); err != nil {
				return err
			}
			// Force model to "script" sentinel.
			updates = append(updates, "model = ?")
			args = append(args, "script")
		}
		updates = append(updates, "execution_mode = ?")
		args = append(args, mode)
	} else if req.PythonScriptID != nil {
		// PythonScriptID set without changing ExecutionMode: validate the existing mode.
		var currentMode string
		queryErr := s.pool.QueryRow(
			"SELECT execution_mode FROM agent_definitions WHERE LOWER(project_id) = LOWER(?) AND LOWER(workflow_id) = LOWER(?) AND LOWER(id) = LOWER(?)",
			projectID, workflowID, id).Scan(&currentMode)
		if queryErr != nil {
			return fmt.Errorf("failed to load agent definition: %w", queryErr)
		}
		if currentMode != "script" {
			return fmt.Errorf("python_script_id_requires_script_mode")
		}
		if s.pythonScriptRepo != nil {
			script, err := s.pythonScriptRepo.Get(projectID, *req.PythonScriptID)
			if err != nil {
				return fmt.Errorf("python_script_not_found: %s", *req.PythonScriptID)
			}
			if script.Kind == "tool" {
				return fmt.Errorf("python_script_kind_mismatch")
			}
		}
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
	if req.PythonScriptID != nil {
		updates = append(updates, "python_script_id = ?")
		args = append(args, *req.PythonScriptID)
	}
	if req.ValidationCommands != nil {
		if err := validateValidationCommands(*req.ValidationCommands); err != nil {
			return err
		}
		b, err := json.Marshal(*req.ValidationCommands)
		if err != nil {
			return fmt.Errorf("failed to marshal validation_commands: %w", err)
		}
		updates = append(updates, "validation_commands = ?")
		args = append(args, string(b))
	}
	// Re-validate the consultant+node_role invariant whenever consultant,
	// execution_mode, node_role, or tools changes (effective values resolved
	// against the current row — see revalidateConsultantAndNodeRole).
	if err := s.revalidateConsultantAndNodeRole(projectID, workflowID, id, req); err != nil {
		return err
	}
	if req.Consultant != nil {
		updates = append(updates, "consultant = ?")
		args = append(args, *req.Consultant)
	}
	if req.NodeRole != nil {
		updates = append(updates, "node_role = ?")
		args = append(args, *req.NodeRole)
	}
	if req.Description != nil {
		updates = append(updates, "description = ?")
		args = append(args, *req.Description)
	}
	if req.ReasoningEffort != nil {
		updates = append(updates, "reasoning_effort = ?")
		args = append(args, *req.ReasoningEffort)
	}

	if len(updates) == 0 {
		return nil
	}

	now := s.clock.Now().UTC().Format(time.RFC3339Nano)
	updates = append(updates, "updated_at = ?")
	args = append(args, now)
	args = append(args, projectID, workflowID, id)

	query := "UPDATE agent_definitions SET " + strings.Join(updates, ", ") +
		" WHERE LOWER(project_id) = LOWER(?) AND LOWER(workflow_id) = LOWER(?) AND LOWER(id) = LOWER(?)"

	result, err := s.pool.Exec(query, args...)
	if err != nil {
		return err
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("agent definition not found: %s", id)
	}
	return nil
}
