package service

import (
	"database/sql"
	"fmt"
	"strings"

	"be/internal/clock"
	"be/internal/db"
)

// PlanTemplate is a fanout_template agent definition a plan node may bind to.
type PlanTemplate struct {
	ID            string `json:"id"`
	Model         string `json:"model"`
	ExecutionMode string `json:"execution_mode"`
	Prompt        string `json:"prompt"`
	Description   string `json:"description"`
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
		`SELECT id, model, execution_mode, prompt, description FROM agent_definitions
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
		if err := rows.Scan(&t.ID, &t.Model, &t.ExecutionMode, &t.Prompt, &t.Description); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, nil
}

// ValidateTemplatesEnabled re-checks that every given template's model is
// enabled right now (definition-time validation can go stale — a model may be
// disabled between draft and approve, and the spawner does not itself refuse
// a disabled cli model, see spawner_util.go DefaultCLIForModel fallback).
// Aggregates every violation into one error naming template+model+mode.
func ValidateTemplatesEnabled(pool *db.Pool, clk clock.Clock, templates []PlanTemplate) error {
	cliSvc := NewCLIModelService(pool, clk)
	apiSvc := NewAPIModelService(pool, clk)

	var problems []string
	for _, t := range templates {
		var valid bool
		var err error
		switch t.ExecutionMode {
		case "api":
			valid, err = apiSvc.IsValidModel(t.Model)
		default:
			valid, err = cliSvc.IsValidModel(t.Model)
		}
		if err != nil {
			return fmt.Errorf("failed to validate model for template %q: %w", t.ID, err)
		}
		if !valid {
			problems = append(problems, fmt.Sprintf(
				"template %q requires %s model %q, which is not enabled on this server; replan using only the templates listed above",
				t.ID, t.ExecutionMode, t.Model))
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("%s", strings.Join(problems, "\n"))
	}
	return nil
}

// EnabledTemplates filters templates down to those whose model is currently
// enabled, so a planner's ${TEMPLATE_LIBRARY} prompt var only ever lists
// usable templates.
func EnabledTemplates(pool *db.Pool, clk clock.Clock, templates []PlanTemplate) []PlanTemplate {
	cliSvc := NewCLIModelService(pool, clk)
	apiSvc := NewAPIModelService(pool, clk)

	out := make([]PlanTemplate, 0, len(templates))
	for _, t := range templates {
		var valid bool
		switch t.ExecutionMode {
		case "api":
			valid, _ = apiSvc.IsValidModel(t.Model)
		default:
			valid, _ = cliSvc.IsValidModel(t.Model)
		}
		if valid {
			out = append(out, t)
		}
	}
	return out
}
