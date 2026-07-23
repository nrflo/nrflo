package repo

import (
	"database/sql"
	"time"

	"be/internal/model"
)

// agentDefColumns is the shared SELECT column list; scanAgentDefRows scans
// rows produced with exactly this list, in this order.
const agentDefColumns = "id, project_id, workflow_id, model, timeout, prompt, restart_threshold, max_fail_restarts, stall_start_timeout_sec, stall_running_timeout_sec, context_budget_tokens, tag, low_consumption_model, layer, execution_mode, tools, native_tools, sandbox, api_max_iterations, api_max_tokens, python_script_id, validation_commands, consultant, node_role, description, reasoning_effort, system_template_id, proactive_restart_threshold_tokens, tier, created_at, updated_at"

func scanAgentDefRows(rows interface {
	Next() bool
	Scan(...interface{}) error
	Close() error
}) ([]*model.AgentDefinition, error) {

	var defs []*model.AgentDefinition
	for rows.Next() {
		def := &model.AgentDefinition{}
		var createdAt, updatedAt string
		var restartThreshold, maxFailRestarts, stallStartTimeout, stallRunningTimeout, contextBudgetTokens, apiMaxIter, apiMaxTokens, proactiveRestartThreshold, tier sql.NullInt64
		var pythonScriptID, reasoningEffort sql.NullString

		err := rows.Scan(
			&def.ID,
			&def.ProjectID,
			&def.WorkflowID,
			&def.Model,
			&def.Timeout,
			&def.Prompt,
			&restartThreshold,
			&maxFailRestarts,
			&stallStartTimeout,
			&stallRunningTimeout,
			&contextBudgetTokens,
			&def.Tag,
			&def.LowConsumptionModel,
			&def.Layer,
			&def.ExecutionMode,
			&def.Tools,
			&def.NativeTools,
			&def.Sandbox,
			&apiMaxIter,
			&apiMaxTokens,
			&pythonScriptID,
			&def.ValidationCommands,
			&def.Consultant,
			&def.NodeRole,
			&def.Description,
			&reasoningEffort,
			&def.SystemTemplateID,
			&proactiveRestartThreshold,
			&tier,
			&createdAt,
			&updatedAt,
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
		if contextBudgetTokens.Valid {
			v := int(contextBudgetTokens.Int64)
			def.ContextBudgetTokens = &v
		}
		if apiMaxIter.Valid {
			v := int(apiMaxIter.Int64)
			def.APIMaxIterations = &v
		}
		if apiMaxTokens.Valid {
			v := int(apiMaxTokens.Int64)
			def.APIMaxTokens = &v
		}
		if pythonScriptID.Valid {
			s := pythonScriptID.String
			def.PythonScriptID = &s
		}
		if reasoningEffort.Valid {
			v := reasoningEffort.String
			def.ReasoningEffort = &v
		}
		if proactiveRestartThreshold.Valid {
			v := int(proactiveRestartThreshold.Int64)
			def.ProactiveRestartThresholdTokens = &v
		}
		if tier.Valid {
			v := int(tier.Int64)
			def.Tier = &v
		}

		defs = append(defs, def)
	}

	return defs, nil
}
