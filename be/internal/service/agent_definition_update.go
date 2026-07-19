package service

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"be/internal/types"
)

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
		if mode != "script" {
			valid, vErr := s.modelSvc.IsValidModelForMode(*req.Model, registryMode(mode))
			if vErr != nil {
				return fmt.Errorf("failed to validate model: %w", vErr)
			}
			if !valid {
				return validationErrorf("invalid model: %q", *req.Model)
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
	if req.ContextBudgetTokens != nil {
		if *req.ContextBudgetTokens < 0 {
			return validationErrorf("context_budget_tokens must be >= 0")
		}
		updates = append(updates, "context_budget_tokens = ?")
		args = append(args, *req.ContextBudgetTokens)
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
			if mode != "script" {
				valid, vErr := s.modelSvc.IsValidModelForMode(lcModel, registryMode(mode))
				if vErr != nil {
					return fmt.Errorf("failed to validate low_consumption_model: %w", vErr)
				}
				if !valid {
					return validationErrorf("invalid low_consumption_model: %q", lcModel)
				}
			}
		}
		updates = append(updates, "low_consumption_model = ?")
		args = append(args, lcModel)
	}
	if req.ExecutionMode != nil {
		mode := *req.ExecutionMode
		if mode != "cli_interactive" && mode != "api" && mode != "script" {
			return validationErrorf("invalid execution_mode: %q", mode)
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
			return validationErrorf("python_script_id_requires_script_mode")
		}
		if s.pythonScriptRepo != nil {
			script, err := s.pythonScriptRepo.Get(projectID, *req.PythonScriptID)
			if err != nil {
				return fmt.Errorf("python_script_not_found: %s", *req.PythonScriptID)
			}
			if script.Kind == "tool" {
				return validationErrorf("python_script_kind_mismatch")
			}
		}
	}
	if req.Tools != nil {
		updates = append(updates, "tools = ?")
		args = append(args, *req.Tools)
	}
	if req.NativeTools != nil {
		normalized, err := normalizeNativeTools(*req.NativeTools)
		if err != nil {
			return err
		}
		req.NativeTools = &normalized
		updates = append(updates, "native_tools = ?")
		args = append(args, normalized)
	}
	if req.Sandbox != nil {
		updates = append(updates, "sandbox = ?")
		args = append(args, *req.Sandbox)
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
	// Same merged-effective-values guard for native_tools/sandbox vs the
	// def's model provider and execution mode.
	if err := s.revalidateNativeFields(projectID, workflowID, id, req); err != nil {
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
	if req.SystemTemplateID != nil {
		if err := s.validateSystemTemplateID(*req.SystemTemplateID); err != nil {
			return err
		}
		updates = append(updates, "system_template_id = ?")
		args = append(args, *req.SystemTemplateID)
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
