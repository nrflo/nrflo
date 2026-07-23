package service

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"be/internal/types"
)

// appendMetaUpdates handles the simple trailing agent_definitions fields plus
// the invariant re-checks that must run whenever any of them changes on a
// PATCH: consultant/node_role/tools coupling, native_tools/sandbox vs model
// provider, and prompt_mode/steps coupling.
func (s *AgentDefinitionService) appendMetaUpdates(projectID, workflowID, id string, req *types.AgentDefUpdateRequest, updates *[]string, args *[]any) error {
	if err := s.revalidateConsultantAndNodeRole(projectID, workflowID, id, req); err != nil {
		return err
	}
	if err := s.revalidateNativeFields(projectID, workflowID, id, req); err != nil {
		return err
	}
	if req.Consultant != nil {
		*updates = append(*updates, "consultant = ?")
		*args = append(*args, *req.Consultant)
	}
	if req.NodeRole != nil {
		*updates = append(*updates, "node_role = ?")
		*args = append(*args, *req.NodeRole)
	}
	if req.Description != nil {
		*updates = append(*updates, "description = ?")
		*args = append(*args, *req.Description)
	}
	if req.ReasoningEffort != nil {
		*updates = append(*updates, "reasoning_effort = ?")
		*args = append(*args, *req.ReasoningEffort)
	}
	if req.SystemTemplateID != nil {
		if err := s.validateSystemTemplateID(*req.SystemTemplateID); err != nil {
			return err
		}
		*updates = append(*updates, "system_template_id = ?")
		*args = append(*args, *req.SystemTemplateID)
	}

	setClauses, promptArgs, err := s.resolvePromptModeUpdate(projectID, workflowID, id, req)
	if err != nil {
		return err
	}
	*updates = append(*updates, setClauses...)
	*args = append(*args, promptArgs...)
	return nil
}

// resolvePromptModeUpdate applies the merged-effective-value rules for
// prompt_mode/steps: a PATCH that omits both fields is a no-op; one that
// omits only prompt_mode is validated against the stored mode; switching to
// 'full' always clears steps; switching to (or staying) 'stepwise' requires
// a non-empty steps array (incoming or already stored) and rejects
// execution_mode='script' (script agents carry no prompt at all).
func (s *AgentDefinitionService) resolvePromptModeUpdate(projectID, workflowID, id string, req *types.AgentDefUpdateRequest) ([]string, []any, error) {
	if req.PromptMode == nil && req.Steps == nil {
		return nil, nil, nil
	}

	var currentMode, currentExecMode string
	var currentSteps sql.NullString
	if err := s.pool.QueryRow(
		"SELECT prompt_mode, steps, execution_mode FROM agent_definitions WHERE LOWER(project_id) = LOWER(?) AND LOWER(workflow_id) = LOWER(?) AND LOWER(id) = LOWER(?)",
		projectID, workflowID, id).Scan(&currentMode, &currentSteps, &currentExecMode); err != nil {
		return nil, nil, fmt.Errorf("failed to load agent definition: %w", err)
	}

	effectiveMode := currentMode
	if req.PromptMode != nil {
		effectiveMode = *req.PromptMode
	}
	if effectiveMode != PromptModeFull && effectiveMode != PromptModeStepwise {
		return nil, nil, validationErrorf("invalid prompt_mode: %q", effectiveMode)
	}
	effectiveExecMode := currentExecMode
	if req.ExecutionMode != nil {
		effectiveExecMode = *req.ExecutionMode
	}

	if effectiveMode == PromptModeFull {
		if req.Steps != nil {
			return nil, nil, validationErrorf("steps require prompt_mode=stepwise")
		}
		if currentMode == PromptModeFull {
			return nil, nil, nil
		}
		return []string{"prompt_mode = ?", "steps = ?"}, []any{PromptModeFull, nil}, nil
	}

	if effectiveExecMode == "script" {
		return nil, nil, validationErrorf("script mode agents cannot use prompt_mode=stepwise")
	}

	if req.Steps == nil {
		if !currentSteps.Valid || currentSteps.String == "" {
			return nil, nil, validationErrorf("prompt_mode=stepwise requires steps")
		}
		if req.PromptMode == nil {
			return nil, nil, nil
		}
		return []string{"prompt_mode = ?"}, []any{PromptModeStepwise}, nil
	}

	steps := *req.Steps
	if len(steps) == 0 {
		return nil, nil, validationErrorf("prompt_mode=stepwise requires at least one step")
	}
	if err := validateStepDefinitions(steps); err != nil {
		return nil, nil, err
	}
	b, err := json.Marshal(steps)
	if err != nil {
		return nil, nil, validationErrorf("failed to marshal steps: %v", err)
	}
	return []string{"prompt_mode = ?", "steps = ?"}, []any{PromptModeStepwise, string(b)}, nil
}
