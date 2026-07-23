package repo

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// AgentDefUpdateFields contains fields that can be updated
type AgentDefUpdateFields struct {
	Model                  *string
	Timeout                *int
	Prompt                 *string
	Layer                  *int
	RestartThreshold       *int
	MaxFailRestarts        *int
	StallStartTimeoutSec   *int
	StallRunningTimeoutSec *int
	ContextBudgetTokens    *int
	Tag                    *string
	LowConsumptionModel    *string
	ExecutionMode          *string
	Tools                  *string
	NativeTools            *string
	Sandbox                *string
	APIMaxIterations       *int
	APIMaxTokens           *int
	PythonScriptID         *string
	ValidationCommands     *string
	Consultant             *bool
	NodeRole               *string
	Description            *string
	// ReasoningEffort: nil = untouched; non-nil with Valid=false writes NULL
	// (revert to inherit-from-model-row); non-nil with Valid=true writes the
	// string (incl. "" for an explicit no-effort override).
	ReasoningEffort                 *sql.NullString
	SystemTemplateID                *string
	ProactiveRestartThresholdTokens *int
	// Tier: nil = untouched; non-nil with Valid=false writes NULL (untier);
	// non-nil with Valid=true writes the tier value.
	Tier *sql.NullInt64
}

// Update updates an agent definition
func (r *AgentDefinitionRepo) Update(projectID, workflowID, id string, fields *AgentDefUpdateFields) error {
	updates := []string{}
	args := []interface{}{}

	if fields.Model != nil {
		updates = append(updates, "model = ?")
		args = append(args, *fields.Model)
	}
	if fields.Timeout != nil {
		updates = append(updates, "timeout = ?")
		args = append(args, *fields.Timeout)
	}
	if fields.Prompt != nil {
		updates = append(updates, "prompt = ?")
		args = append(args, *fields.Prompt)
	}
	if fields.Layer != nil {
		updates = append(updates, "layer = ?")
		args = append(args, *fields.Layer)
	}
	if fields.RestartThreshold != nil {
		updates = append(updates, "restart_threshold = ?")
		args = append(args, *fields.RestartThreshold)
	}
	if fields.MaxFailRestarts != nil {
		updates = append(updates, "max_fail_restarts = ?")
		args = append(args, *fields.MaxFailRestarts)
	}
	if fields.StallStartTimeoutSec != nil {
		updates = append(updates, "stall_start_timeout_sec = ?")
		args = append(args, *fields.StallStartTimeoutSec)
	}
	if fields.StallRunningTimeoutSec != nil {
		updates = append(updates, "stall_running_timeout_sec = ?")
		args = append(args, *fields.StallRunningTimeoutSec)
	}
	if fields.ContextBudgetTokens != nil {
		updates = append(updates, "context_budget_tokens = ?")
		args = append(args, *fields.ContextBudgetTokens)
	}
	if fields.Tag != nil {
		updates = append(updates, "tag = ?")
		args = append(args, *fields.Tag)
	}
	if fields.LowConsumptionModel != nil {
		updates = append(updates, "low_consumption_model = ?")
		args = append(args, *fields.LowConsumptionModel)
	}
	if fields.ExecutionMode != nil {
		updates = append(updates, "execution_mode = ?")
		args = append(args, *fields.ExecutionMode)
	}
	if fields.Tools != nil {
		updates = append(updates, "tools = ?")
		args = append(args, *fields.Tools)
	}
	if fields.NativeTools != nil {
		updates = append(updates, "native_tools = ?")
		args = append(args, *fields.NativeTools)
	}
	if fields.Sandbox != nil {
		updates = append(updates, "sandbox = ?")
		args = append(args, *fields.Sandbox)
	}
	if fields.APIMaxIterations != nil {
		updates = append(updates, "api_max_iterations = ?")
		args = append(args, *fields.APIMaxIterations)
	}
	if fields.APIMaxTokens != nil {
		updates = append(updates, "api_max_tokens = ?")
		args = append(args, *fields.APIMaxTokens)
	}
	if fields.PythonScriptID != nil {
		updates = append(updates, "python_script_id = ?")
		args = append(args, *fields.PythonScriptID)
	}
	if fields.ValidationCommands != nil {
		updates = append(updates, "validation_commands = ?")
		args = append(args, *fields.ValidationCommands)
	}
	if fields.Consultant != nil {
		updates = append(updates, "consultant = ?")
		args = append(args, *fields.Consultant)
	}
	if fields.NodeRole != nil {
		updates = append(updates, "node_role = ?")
		args = append(args, *fields.NodeRole)
	}
	if fields.Description != nil {
		updates = append(updates, "description = ?")
		args = append(args, *fields.Description)
	}
	if fields.ReasoningEffort != nil {
		updates = append(updates, "reasoning_effort = ?")
		if fields.ReasoningEffort.Valid {
			args = append(args, fields.ReasoningEffort.String)
		} else {
			args = append(args, nil)
		}
	}

	if fields.SystemTemplateID != nil {
		updates = append(updates, "system_template_id = ?")
		args = append(args, *fields.SystemTemplateID)
	}
	if fields.ProactiveRestartThresholdTokens != nil {
		updates = append(updates, "proactive_restart_threshold_tokens = ?")
		args = append(args, *fields.ProactiveRestartThresholdTokens)
	}
	if fields.Tier != nil {
		updates = append(updates, "tier = ?")
		if fields.Tier.Valid {
			args = append(args, fields.Tier.Int64)
		} else {
			args = append(args, nil)
		}
	}

	if len(updates) == 0 {
		return nil
	}

	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
	updates = append(updates, "updated_at = ?")
	args = append(args, now)
	args = append(args, projectID, workflowID, id)

	query := "UPDATE agent_definitions SET " + strings.Join(updates, ", ") +
		" WHERE LOWER(project_id) = LOWER(?) AND LOWER(workflow_id) = LOWER(?) AND LOWER(id) = LOWER(?)"

	result, err := r.db.Exec(query, args...)
	if err != nil {
		return err
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("agent definition not found: %s/%s/%s", projectID, workflowID, id)
	}
	return nil
}
