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

// ErrValidation marks user-input validation failures (bad model, effort,
// execution_mode, node_role, layer, …) so API handlers can map them to HTTP 400
// via errors.Is(err, ErrValidation) instead of falling through to 500.
var ErrValidation = errors.New("validation")

// validationErr wraps a validation message while unwrapping to ErrValidation.
// The message text is left exactly as callers wrote it, so tests that assert on
// message content keep passing.
type validationErr struct{ msg string }

func (e *validationErr) Error() string { return e.msg }
func (e *validationErr) Unwrap() error { return ErrValidation }

// validationErrorf builds a user-input validation error tagged with ErrValidation.
func validationErrorf(format string, a ...any) error {
	return &validationErr{msg: fmt.Sprintf(format, a...)}
}

// AgentDefinitionService handles agent definition business logic
type AgentDefinitionService struct {
	clock            clock.Clock
	pool             *db.Pool
	modelSvc         *ModelService
	pythonScriptRepo *repo.PythonScriptRepo
}

// NewAgentDefinitionService creates a new agent definition service
func NewAgentDefinitionService(pool *db.Pool, clk clock.Clock, modelSvc *ModelService, pythonScriptRepo *repo.PythonScriptRepo) *AgentDefinitionService {
	return &AgentDefinitionService{pool: pool, clock: clk, modelSvc: modelSvc, pythonScriptRepo: pythonScriptRepo}
}

// CreateAgentDef creates a new agent definition
func (s *AgentDefinitionService) CreateAgentDef(projectID, workflowID string, req *types.AgentDefCreateRequest) (*model.AgentDefinition, error) {
	if req.ID == "" {
		return nil, validationErrorf("agent id is required")
	}

	// Determine execution mode early so we can skip prompt requirement for scripts.
	executionMode := req.ExecutionMode
	if executionMode == "" {
		executionMode = "cli_interactive"
	}
	if executionMode != "cli_interactive" && executionMode != "api" && executionMode != "script" {
		return nil, validationErrorf("invalid execution_mode: %q", executionMode)
	}
	if executionMode == "api" {
		settingsSvc := NewGlobalSettingsService(s.pool, s.clock)
		apiModeVal, _ := settingsSvc.Get("api_mode_enabled")
		if apiModeVal != "true" {
			return nil, ErrAPIModeDisabled
		}
	}

	if req.Consultant && executionMode != "api" {
		return nil, validationErrorf("consultant agent requires execution_mode=api")
	}

	nodeRole, err0 := validateNodeRole(req.NodeRole, req.Consultant, req.Tools)
	if err0 != nil {
		return nil, err0
	}
	if err := validateDescription(nodeRole, req.Description); err != nil {
		return nil, err
	}

	if executionMode != "script" && req.Prompt == "" {
		return nil, validationErrorf("prompt is required")
	}

	// Script mode: enforce coupling rules.
	if executionMode == "script" {
		if err := s.validateScriptMode(projectID, req.PythonScriptID, req.Prompt, req.Tools, req.APIMaxIterations, req.APIMaxTokens); err != nil {
			return nil, err
		}
	} else if req.PythonScriptID != nil {
		return nil, validationErrorf("python_script_id_requires_script_mode")
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
		valid, err := s.modelSvc.IsValidModelForMode(lcModel, registryMode(executionMode))
		if err != nil {
			return nil, fmt.Errorf("failed to validate low_consumption_model: %w", err)
		}
		if !valid {
			return nil, validationErrorf("invalid low_consumption_model: %q", lcModel)
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
		modelName = "sonnet-5"
	}

	if executionMode != "script" {
		valid, err := s.modelSvc.IsValidModelForMode(modelName, registryMode(executionMode))
		if err != nil {
			return nil, fmt.Errorf("failed to validate model: %w", err)
		}
		if !valid {
			return nil, validationErrorf("invalid model: %q", modelName)
		}
	}

	if err := validateDefReasoningEffort(s.modelSvc, executionMode, modelName, req.ReasoningEffort); err != nil {
		return nil, err
	}

	if err := s.validateSystemTemplateID(req.SystemTemplateID); err != nil {
		return nil, err
	}

	nativeTools, nErr := normalizeNativeTools(req.NativeTools)
	if nErr != nil {
		return nil, nErr
	}
	if err := validateNativeFields(s.modelSvc, executionMode, modelName, nativeTools, req.Sandbox); err != nil {
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
		INSERT INTO agent_definitions (id, project_id, workflow_id, model, timeout, prompt, restart_threshold, max_fail_restarts, stall_start_timeout_sec, stall_running_timeout_sec, tag, low_consumption_model, layer, execution_mode, tools, native_tools, sandbox, api_max_iterations, api_max_tokens, python_script_id, validation_commands, consultant, node_role, description, reasoning_effort, system_template_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, pid, wid, modelName, timeout, req.Prompt, req.RestartThreshold, req.MaxFailRestarts, stallStartTimeout, req.StallRunningTimeoutSec, req.Tag, lcModel, req.Layer, executionMode, req.Tools, nativeTools, req.Sandbox, req.APIMaxIterations, req.APIMaxTokens, req.PythonScriptID, validationCommandsJSON, req.Consultant, nodeRole, req.Description, req.ReasoningEffort, req.SystemTemplateID, now, now,
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
		NativeTools:            nativeTools,
		Sandbox:                req.Sandbox,
		APIMaxIterations:       req.APIMaxIterations,
		APIMaxTokens:           req.APIMaxTokens,
		PythonScriptID:         req.PythonScriptID,
		ValidationCommands:     validationCommandsJSON,
		ReasoningEffort:        req.ReasoningEffort,
		SystemTemplateID:       req.SystemTemplateID,
		Consultant:             req.Consultant,
		NodeRole:               nodeRole,
		Description:            req.Description,
		CreatedAt:              ts,
		UpdatedAt:              ts,
	}, nil
}

// validateSystemTemplateID allows an empty id (mode default / global override
// gate) and otherwise requires it to resolve to an injectable default_templates row.
func (s *AgentDefinitionService) validateSystemTemplateID(id string) error {
	if id == "" {
		return nil
	}
	var count int
	if err := s.pool.QueryRow(
		"SELECT COUNT(*) FROM default_templates WHERE id = ? AND type = 'injectable'", id,
	).Scan(&count); err != nil {
		return fmt.Errorf("failed to validate system_template_id: %w", err)
	}
	if count == 0 {
		return validationErrorf("invalid system_template_id: %q", id)
	}
	return nil
}
