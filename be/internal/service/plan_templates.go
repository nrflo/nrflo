package service

import (
	"database/sql"
	"fmt"
	"strings"

	"be/internal/clock"
	"be/internal/db"
)

// PlanTemplate is a fanout_template agent definition a plan node may bind to.
// ReasoningEffort and CLIType are populated by EnabledTemplates/
// ValidateTemplatesEnabled (empty until then): ReasoningEffort is the
// EFFECTIVE effort (the def's own override when set, else the model row's),
// CLIType is the model row's cli_type (cli_interactive templates only).
type PlanTemplate struct {
	ID              string  `json:"id"`
	Model           string  `json:"model"`
	ExecutionMode   string  `json:"execution_mode"`
	Prompt          string  `json:"prompt"`
	Description     string  `json:"description"`
	ReasoningEffort string  `json:"reasoning_effort"`
	CLIType         string  `json:"cli_type,omitempty"`
	effortOverride  *string // def-level override, nil = inherit from the model row
}

// AllowedTemplates returns the fanout_template agent definitions usable by a
// plan for (projectID, workflowID). Resolution mirrors
// orchestrator.resolveWorkflowDef: the workflow definition is looked up under
// the selected project, falling back to GlobalProjectID when not found there,
// and templates are read from whichever project the definition resolved
// under (a global workflow's templates live under __global__ too).
func AllowedTemplates(pool *db.Pool, projectID, workflowID string) ([]PlanTemplate, error) {
	defProjectID := projectID
	var exists int
	err := pool.QueryRow(
		`SELECT 1 FROM workflows WHERE LOWER(project_id) = LOWER(?) AND LOWER(id) = LOWER(?)`,
		projectID, workflowID).Scan(&exists)
	if err == sql.ErrNoRows {
		defProjectID = GlobalProjectID
	} else if err != nil {
		return nil, err
	}

	rows, err := pool.Query(
		`SELECT id, model, execution_mode, prompt, description, reasoning_effort FROM agent_definitions
		 WHERE LOWER(project_id) = LOWER(?) AND LOWER(workflow_id) = LOWER(?)
		   AND node_role = 'fanout_template' AND consultant = 0
		 ORDER BY id`, defProjectID, workflowID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PlanTemplate
	for rows.Next() {
		var t PlanTemplate
		var effort sql.NullString
		if err := rows.Scan(&t.ID, &t.Model, &t.ExecutionMode, &t.Prompt, &t.Description, &effort); err != nil {
			return nil, err
		}
		if effort.Valid {
			v := effort.String
			t.effortOverride = &v
		}
		out = append(out, t)
	}
	return out, nil
}

// resolveTemplateAvailability resolves the model row for a template and
// reports whether it can actually run on this install right now: the model
// row must be enabled, and — for cli_interactive templates — the cli binary
// must be present on PATH (read_only rows, e.g. every seeded codex_* row,
// can never be disabled via the `enabled` flag, so PATH probing is the only
// way to hide them on an install that lacks the binary), and — for api
// templates — api_mode_enabled must be on. Returns the effective reasoning
// effort (def override if set, else the model row's) and cli_type
// (cli_interactive only) alongside the availability verdict.
func resolveTemplateAvailability(cliSvc *CLIModelService, apiSvc *APIModelService, apiModeEnabled bool, t PlanTemplate) (available bool, effort, cliType string) {
	if t.ExecutionMode == "api" {
		if !apiModeEnabled {
			return false, "", ""
		}
		am, err := apiSvc.Get(t.Model)
		if err != nil || !am.Enabled {
			return false, "", ""
		}
		effort = am.ReasoningEffort
		if t.effortOverride != nil {
			effort = *t.effortOverride
		}
		return true, effort, ""
	}

	cm, err := cliSvc.Get(t.Model)
	if err != nil || !cm.Enabled {
		return false, "", ""
	}
	if !CLIAvailable(cm.CLIType) {
		return false, "", ""
	}
	effort = cm.ReasoningEffort
	if t.effortOverride != nil {
		effort = *t.effortOverride
	}
	return true, effort, cm.CLIType
}

// ValidateTemplatesEnabled re-checks that every given template can actually
// run right now (definition-time validation can go stale — a model may be
// disabled between draft and approve, a binary may be absent, or api mode may
// be off). Aggregates every violation into one error naming template+model+mode.
func ValidateTemplatesEnabled(pool *db.Pool, clk clock.Clock, templates []PlanTemplate) error {
	cliSvc := NewCLIModelService(pool, clk)
	apiSvc := NewAPIModelService(pool, clk)
	apiModeEnabled := apiModeEnabledSetting(pool, clk)

	var problems []string
	for _, t := range templates {
		if available, _, _ := resolveTemplateAvailability(cliSvc, apiSvc, apiModeEnabled, t); !available {
			problems = append(problems, fmt.Sprintf(
				"template %q requires %s model %q, which is not available on this server; replan using only the templates listed above",
				t.ID, t.ExecutionMode, t.Model))
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("%s", strings.Join(problems, "\n"))
	}
	return nil
}

// EnabledTemplates filters templates down to those currently available (model
// enabled, cli binary present, api mode on as applicable) and fills in the
// effective reasoning effort + cli_type, so a planner's ${TEMPLATE_LIBRARY}
// prompt var only ever lists usable templates.
func EnabledTemplates(pool *db.Pool, clk clock.Clock, templates []PlanTemplate) []PlanTemplate {
	cliSvc := NewCLIModelService(pool, clk)
	apiSvc := NewAPIModelService(pool, clk)
	apiModeEnabled := apiModeEnabledSetting(pool, clk)

	out := make([]PlanTemplate, 0, len(templates))
	for _, t := range templates {
		available, effort, cliType := resolveTemplateAvailability(cliSvc, apiSvc, apiModeEnabled, t)
		if !available {
			continue
		}
		t.ReasoningEffort = effort
		t.CLIType = cliType
		out = append(out, t)
	}
	return out
}

// apiModeEnabledSetting reads the api_mode_enabled global setting.
func apiModeEnabledSetting(pool *db.Pool, clk clock.Clock) bool {
	settingsSvc := NewGlobalSettingsService(pool, clk)
	v, _ := settingsSvc.Get("api_mode_enabled")
	return v == "true"
}
