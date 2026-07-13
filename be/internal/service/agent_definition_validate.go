package service

import (
	"fmt"
	"strings"
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
// as workflow phases).
func validateNodeRole(role string, consultant bool) (string, error) {
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
	return role, nil
}

// validateConsultantAndNodeRole re-validates the consultant+execution_mode+node_role
// invariant against the effective values (current row merged with the incoming
// update), so a PATCH that omits a field cannot violate the invariant.
func validateConsultantAndNodeRole(consultant bool, executionMode, nodeRole string) error {
	if consultant && executionMode != "api" {
		return fmt.Errorf("consultant agent requires execution_mode=api")
	}
	if _, err := validateNodeRole(nodeRole, consultant); err != nil {
		return err
	}
	return nil
}
