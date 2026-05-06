package service

import (
	"database/sql"
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

// ErrAPIModeDisabled is returned when execution_mode="api" is used but the server was not started with --mode=api.
var ErrAPIModeDisabled = errors.New("api mode disabled")

// AgentDefinitionService handles agent definition business logic
type AgentDefinitionService struct {
	clock            clock.Clock
	pool             *db.Pool
	cliModelSvc      *CLIModelService
	pythonScriptRepo *repo.PythonScriptRepo
	apiMode          bool
}

// NewAgentDefinitionService creates a new agent definition service
func NewAgentDefinitionService(pool *db.Pool, clk clock.Clock, cliModelSvc *CLIModelService, pythonScriptRepo *repo.PythonScriptRepo, apiMode bool) *AgentDefinitionService {
	return &AgentDefinitionService{pool: pool, clock: clk, cliModelSvc: cliModelSvc, pythonScriptRepo: pythonScriptRepo, apiMode: apiMode}
}

// validateScriptMode enforces coupling rules for execution_mode="script":
// PythonScriptID required, Prompt/Tools/APIMaxIterations must be empty/nil,
// script must belong to the given project.
func (s *AgentDefinitionService) validateScriptMode(projectID string, pythonScriptID *string, prompt, tools string, apiMaxIterations *int) error {
	if pythonScriptID == nil {
		return fmt.Errorf("python_script_id_required")
	}
	if prompt != "" {
		return fmt.Errorf("script_mode_no_prompt")
	}
	if tools != "" {
		return fmt.Errorf("script_mode_no_tools")
	}
	if apiMaxIterations != nil {
		return fmt.Errorf("script_mode_no_api_max_iterations")
	}
	if s.pythonScriptRepo != nil {
		if _, err := s.pythonScriptRepo.Get(projectID, *pythonScriptID); err != nil {
			return fmt.Errorf("python_script_not_found: %s", *pythonScriptID)
		}
	}
	return nil
}

// CreateAgentDef creates a new agent definition
func (s *AgentDefinitionService) CreateAgentDef(projectID, workflowID string, req *types.AgentDefCreateRequest) (*model.AgentDefinition, error) {
	if req.ID == "" {
		return nil, fmt.Errorf("agent id is required")
	}

	// Determine execution mode early so we can skip prompt requirement for scripts.
	executionMode := req.ExecutionMode
	if executionMode == "" {
		executionMode = "cli"
	}
	if executionMode != "cli" && executionMode != "api" && executionMode != "script" {
		return nil, fmt.Errorf("invalid execution_mode: %q", executionMode)
	}
	if executionMode == "api" && !s.apiMode {
		return nil, ErrAPIModeDisabled
	}

	if executionMode != "script" && req.Prompt == "" {
		return nil, fmt.Errorf("prompt is required")
	}

	// Script mode: enforce coupling rules.
	if executionMode == "script" {
		if err := s.validateScriptMode(projectID, req.PythonScriptID, req.Prompt, req.Tools, req.APIMaxIterations); err != nil {
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

	// Validate low_consumption_model against cli_models DB table
	lcModel := strings.ToLower(req.LowConsumptionModel)
	if lcModel != "" {
		valid, err := s.cliModelSvc.IsValidModel(lcModel)
		if err != nil {
			return nil, fmt.Errorf("failed to validate low_consumption_model: %w", err)
		}
		if !valid {
			return nil, fmt.Errorf("invalid low_consumption_model: %q", lcModel)
		}
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
		INSERT INTO agent_definitions (id, project_id, workflow_id, model, timeout, prompt, restart_threshold, max_fail_restarts, stall_start_timeout_sec, stall_running_timeout_sec, tag, low_consumption_model, layer, execution_mode, tools, api_max_iterations, python_script_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, pid, wid, modelName, timeout, req.Prompt, req.RestartThreshold, req.MaxFailRestarts, stallStartTimeout, req.StallRunningTimeoutSec, req.Tag, lcModel, req.Layer, executionMode, req.Tools, req.APIMaxIterations, req.PythonScriptID, now, now,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") || strings.Contains(err.Error(), "already exists") {
			return nil, fmt.Errorf("agent definition already exists: %s", req.ID)
		}
		return nil, err
	}

	ts, _ := time.Parse(time.RFC3339Nano, now)
	return &model.AgentDefinition{
		ID:               id,
		ProjectID:        pid,
		WorkflowID:       wid,
		Model:            modelName,
		Timeout:          timeout,
		Prompt:           req.Prompt,
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
		PythonScriptID:         req.PythonScriptID,
		CreatedAt:              ts,
		UpdatedAt:              ts,
	}, nil
}

// GetAgentDef retrieves a single agent definition
func (s *AgentDefinitionService) GetAgentDef(projectID, workflowID, id string) (*model.AgentDefinition, error) {
	def := &model.AgentDefinition{}
	var createdAt, updatedAt string
	var restartThreshold, maxFailRestarts, stallStartTimeout, stallRunningTimeout, apiMaxIter sql.NullInt64
	var pythonScriptID sql.NullString

	err := s.pool.QueryRow(`
		SELECT id, project_id, workflow_id, model, timeout, prompt, restart_threshold, max_fail_restarts, stall_start_timeout_sec, stall_running_timeout_sec, tag, low_consumption_model, layer, execution_mode, tools, api_max_iterations, python_script_id, created_at, updated_at
		FROM agent_definitions
		WHERE LOWER(project_id) = LOWER(?) AND LOWER(workflow_id) = LOWER(?) AND LOWER(id) = LOWER(?)`,
		projectID, workflowID, id).Scan(
		&def.ID, &def.ProjectID, &def.WorkflowID,
		&def.Model, &def.Timeout, &def.Prompt,
		&restartThreshold, &maxFailRestarts, &stallStartTimeout, &stallRunningTimeout, &def.Tag,
		&def.LowConsumptionModel, &def.Layer,
		&def.ExecutionMode, &def.Tools, &apiMaxIter, &pythonScriptID,
		&createdAt, &updatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("agent definition not found: %s", id)
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
	if apiMaxIter.Valid {
		v := int(apiMaxIter.Int64)
		def.APIMaxIterations = &v
	}
	if pythonScriptID.Valid {
		s := pythonScriptID.String
		def.PythonScriptID = &s
	}
	return def, nil
}

// ListAgentDefs retrieves all agent definitions for a workflow
func (s *AgentDefinitionService) ListAgentDefs(projectID, workflowID string) ([]*model.AgentDefinition, error) {
	rows, err := s.pool.Query(`
		SELECT id, project_id, workflow_id, model, timeout, prompt, restart_threshold, max_fail_restarts, stall_start_timeout_sec, stall_running_timeout_sec, tag, low_consumption_model, layer, execution_mode, tools, api_max_iterations, python_script_id, created_at, updated_at
		FROM agent_definitions
		WHERE LOWER(project_id) = LOWER(?) AND LOWER(workflow_id) = LOWER(?)
		ORDER BY layer ASC, id ASC`, projectID, workflowID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	defs := []*model.AgentDefinition{}
	for rows.Next() {
		def := &model.AgentDefinition{}
		var createdAt, updatedAt string
		var restartThreshold, maxFailRestarts, stallStartTimeout, stallRunningTimeout, apiMaxIter sql.NullInt64
		var pythonScriptID sql.NullString

		err := rows.Scan(
			&def.ID, &def.ProjectID, &def.WorkflowID,
			&def.Model, &def.Timeout, &def.Prompt,
			&restartThreshold, &maxFailRestarts, &stallStartTimeout, &stallRunningTimeout, &def.Tag,
			&def.LowConsumptionModel, &def.Layer,
			&def.ExecutionMode, &def.Tools, &apiMaxIter, &pythonScriptID,
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
		if apiMaxIter.Valid {
			v := int(apiMaxIter.Int64)
			def.APIMaxIterations = &v
		}
		if pythonScriptID.Valid {
			s := pythonScriptID.String
			def.PythonScriptID = &s
		}
		defs = append(defs, def)
	}

	return defs, nil
}

// UpdateAgentDef updates an agent definition
func (s *AgentDefinitionService) UpdateAgentDef(projectID, workflowID, id string, req *types.AgentDefUpdateRequest) error {
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
				   AND layer = ? AND LOWER(id) != LOWER(?)`,
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
			valid, err := s.cliModelSvc.IsValidModel(lcModel)
			if err != nil {
				return fmt.Errorf("failed to validate low_consumption_model: %w", err)
			}
			if !valid {
				return fmt.Errorf("invalid low_consumption_model: %q", lcModel)
			}
		}
		updates = append(updates, "low_consumption_model = ?")
		args = append(args, lcModel)
	}
	if req.ExecutionMode != nil {
		mode := *req.ExecutionMode
		if mode != "cli" && mode != "api" && mode != "script" {
			return fmt.Errorf("invalid execution_mode: %q", mode)
		}
		if mode == "api" && !s.apiMode {
			return ErrAPIModeDisabled
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
			if err := s.validateScriptMode(projectID, req.PythonScriptID, prompt, tools, req.APIMaxIterations); err != nil {
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
			if _, err := s.pythonScriptRepo.Get(projectID, *req.PythonScriptID); err != nil {
				return fmt.Errorf("python_script_not_found: %s", *req.PythonScriptID)
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
	if req.PythonScriptID != nil {
		updates = append(updates, "python_script_id = ?")
		args = append(args, *req.PythonScriptID)
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
