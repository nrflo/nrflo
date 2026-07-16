package service

import (
	"database/sql"
	"fmt"
	"strings"

	"be/internal/types"
)

// validateScriptMode enforces coupling rules for execution_mode="script":
// PythonScriptID required, Prompt/Tools/APIMaxIterations/APIMaxTokens must be
// empty/nil, script must belong to the given project.
func (s *AgentDefinitionService) validateScriptMode(projectID string, pythonScriptID *string, prompt, tools string, apiMaxIterations, apiMaxTokens *int) error {
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
	if apiMaxTokens != nil {
		return fmt.Errorf("script_mode_no_api_max_tokens")
	}
	if s.pythonScriptRepo != nil {
		script, err := s.pythonScriptRepo.Get(projectID, *pythonScriptID)
		if err != nil {
			return fmt.Errorf("python_script_not_found: %s", *pythonScriptID)
		}
		if script.Kind == "tool" {
			return fmt.Errorf("python_script_kind_mismatch")
		}
	}
	return nil
}

// validateExecutionMode validates a system_agent_definitions execution_mode value.
func validateExecutionMode(mode string) error {
	if mode != "cli_interactive" && mode != "api" {
		return fmt.Errorf("invalid execution_mode: must be 'cli_interactive' or 'api'")
	}
	return nil
}

func validateValidationCommands(cmds []string) error {
	if len(cmds) > 20 {
		return fmt.Errorf("validation_commands: too many entries (max 20)")
	}
	for _, cmd := range cmds {
		if strings.TrimSpace(cmd) == "" {
			return fmt.Errorf("validation_commands: empty or whitespace-only entry")
		}
		if len(cmd) > 1024 {
			return fmt.Errorf("validation_commands: entry exceeds 1024 bytes")
		}
	}
	return nil
}

// validateNodeRole validates node_role: static|planner|fanout_template, empty
// defaults to static. A consultant def must stay static (consultant defs are
// already forced to execution_mode=api, and only static defs auto-execute
// as workflow phases). A planner def must carry a tools CSV that grants
// emit_findings — without it a planner could never surface its manifest.
func validateNodeRole(role string, consultant bool, toolsCSV string) (string, error) {
	if role == "" {
		role = "static"
	}
	switch role {
	case "static", "planner", "fanout_template":
	default:
		return "", fmt.Errorf("invalid node_role: %q", role)
	}
	if consultant && role != "static" {
		return "", fmt.Errorf("consultant agent requires node_role=static")
	}
	if role == "planner" && !csvGrantsTool(toolsCSV, "emit_findings") {
		return "", fmt.Errorf("planner agent requires the emit_findings tool in its tools CSV")
	}
	return role, nil
}

// validateDescription requires a non-empty description for fanout_template
// defs — it is the load-bearing text a planner (and the plan UI) uses to pick
// a template, so an undescribed template is effectively unusable.
func validateDescription(nodeRole, description string) error {
	if nodeRole == "fanout_template" && strings.TrimSpace(description) == "" {
		return fmt.Errorf("fanout_template agent requires a non-empty description")
	}
	return nil
}

// validateDefReasoningEffort loads the def's model row (api or cli, per
// execution_mode) and validates the effort override against the row's
// supported_efforts list. nil effort is always valid (inherit from the model
// row). A model row that no longer exists is left to the caller's own model
// validation.
func validateDefReasoningEffort(cliSvc *CLIModelService, apiSvc *APIModelService, executionMode, modelName string, effort *string) error {
	if effort == nil {
		return nil
	}
	if executionMode == "api" {
		am, err := apiSvc.Get(modelName)
		if err != nil {
			return nil
		}
		return ValidateEffortAllowed(*effort, am.SupportedEfforts)
	}
	cm, err := cliSvc.Get(modelName)
	if err != nil {
		return nil
	}
	return ValidateEffortAllowed(*effort, cm.SupportedEfforts)
}

// validateConsultantAndNodeRole re-validates the consultant+execution_mode+node_role
// invariant against the effective values (current row merged with the incoming
// update), so a PATCH that omits a field cannot violate the invariant.
func validateConsultantAndNodeRole(consultant bool, executionMode, nodeRole, toolsCSV, description string) error {
	if consultant && executionMode != "api" {
		return fmt.Errorf("consultant agent requires execution_mode=api")
	}
	if _, err := validateNodeRole(nodeRole, consultant, toolsCSV); err != nil {
		return err
	}
	return validateDescription(nodeRole, description)
}

// revalidateConsultantAndNodeRole re-checks the consultant+execution_mode+
// node_role+tools invariant, and the reasoning_effort override against the
// effective model row, against the effective values (current row merged with
// the incoming update) whenever any of those fields change on a PATCH — so
// flipping execution_mode away from "api" on an existing consultant, or
// stripping emit_findings from a planner's tools, is rejected even when the
// request does not touch every field. Extending the guard to model+
// reasoning_effort closes the PATCH-safety-net gap where changing only the
// model could strand a reasoning_effort override that is illegal for the new
// model row.
func (s *AgentDefinitionService) revalidateConsultantAndNodeRole(projectID, workflowID, id string, req *types.AgentDefUpdateRequest) error {
	if req.Consultant == nil && req.ExecutionMode == nil && req.NodeRole == nil && req.Tools == nil &&
		req.Description == nil && req.Model == nil && req.ReasoningEffort == nil {
		return nil
	}
	var currentConsultant bool
	var currentMode, currentNodeRole, currentTools, currentDescription, currentModel string
	var currentReasoningEffort sql.NullString
	if queryErr := s.pool.QueryRow(
		"SELECT consultant, execution_mode, node_role, tools, description, model, reasoning_effort FROM agent_definitions WHERE LOWER(project_id) = LOWER(?) AND LOWER(workflow_id) = LOWER(?) AND LOWER(id) = LOWER(?)",
		projectID, workflowID, id).Scan(&currentConsultant, &currentMode, &currentNodeRole, &currentTools, &currentDescription, &currentModel, &currentReasoningEffort); queryErr != nil {
		return fmt.Errorf("failed to load agent definition: %w", queryErr)
	}
	effectiveConsultant := currentConsultant
	if req.Consultant != nil {
		effectiveConsultant = *req.Consultant
	}
	effectiveMode := currentMode
	if req.ExecutionMode != nil {
		effectiveMode = *req.ExecutionMode
	}
	effectiveNodeRole := currentNodeRole
	if req.NodeRole != nil {
		effectiveNodeRole = *req.NodeRole
	}
	effectiveTools := currentTools
	if req.Tools != nil {
		effectiveTools = *req.Tools
	}
	effectiveDescription := currentDescription
	if req.Description != nil {
		effectiveDescription = *req.Description
	}
	if err := validateConsultantAndNodeRole(effectiveConsultant, effectiveMode, effectiveNodeRole, effectiveTools, effectiveDescription); err != nil {
		return err
	}

	effectiveModel := currentModel
	if req.Model != nil {
		effectiveModel = *req.Model
	}
	var effectiveEffort *string
	if currentReasoningEffort.Valid {
		v := currentReasoningEffort.String
		effectiveEffort = &v
	}
	if req.ReasoningEffort != nil {
		effectiveEffort = req.ReasoningEffort
	}
	return validateDefReasoningEffort(s.cliModelSvc, s.apiModelSvc, effectiveMode, effectiveModel, effectiveEffort)
}

// revalidatePlannerTools re-checks that a system agent def whose effective
// role (current row merged with the incoming update) is 'planner' still
// grants emit_findings in its effective tools CSV.
func (s *SystemAgentDefinitionService) revalidatePlannerTools(id string, req *types.SystemAgentDefUpdateRequest) error {
	if req.Role == nil && req.Tools == nil {
		return nil
	}
	var currentRole, currentTools string
	if scanErr := s.pool.QueryRow(
		"SELECT role, tools FROM system_agent_definitions WHERE LOWER(id) = LOWER(?)", id,
	).Scan(&currentRole, &currentTools); scanErr != nil {
		return fmt.Errorf("failed to load system agent definition: %w", scanErr)
	}
	effectiveRole := currentRole
	if req.Role != nil {
		effectiveRole = *req.Role
	}
	effectiveTools := currentTools
	if req.Tools != nil {
		effectiveTools = *req.Tools
	}
	if effectiveRole == "planner" && !csvGrantsTool(effectiveTools, "emit_findings") {
		return fmt.Errorf("planner agent requires the emit_findings tool in its tools CSV")
	}
	return nil
}

// csvGrantsTool reports whether a tools CSV grants the named tool, using the
// same glob semantics as apirun.ResolveRegistry/MatchName ("*" = all,
// "prefix*" = prefix match, otherwise exact match). service cannot import
// spawner/apirun (apirun/tools_builtin imports service — that would cycle),
// so the matcher is duplicated here as a tiny leaf helper. `name` stays a
// parameter to keep the port faithful to those glob semantics even though
// emit_findings is currently the only tool any caller checks.
func csvGrantsTool(csv, name string) bool { //nolint:unparam // general matcher: mirrors apirun.MatchName
	for _, pat := range strings.Split(csv, ",") {
		pat = strings.TrimSpace(pat)
		if pat == "" {
			continue
		}
		if pat == "*" {
			return true
		}
		if strings.HasSuffix(pat, "*") {
			if strings.HasPrefix(name, strings.TrimSuffix(pat, "*")) {
				return true
			}
			continue
		}
		if pat == name {
			return true
		}
	}
	return false
}
