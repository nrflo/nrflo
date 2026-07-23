package orchestrator

import (
	"fmt"
	"strings"

	"be/internal/service"
	"be/internal/types"
)

// renderTemplateLibrary formats the enabled fanout_template defs for the
// ${TEMPLATE_LIBRARY} prompt var, so the planner only ever sees usable
// templates (a disabled model is filtered out, not just flagged). The
// description (not the prompt body) is the selection surface: it is the
// load-bearing text an operator writes to tell the planner what a template
// does and which finding key it emits to.
func renderTemplateLibrary(templates []service.PlanTemplate) string {
	if len(templates) == 0 {
		return "_No templates configured for this workflow — the plan cannot include any nodes._"
	}
	var b strings.Builder
	for _, t := range templates {
		desc := strings.TrimSpace(t.Description)
		if desc == "" {
			desc = "(no description provided)"
		}
		fmt.Fprintf(&b, "- %s (%s, %s, effort=%s)\n  %s\n", t.ID, t.Model, t.ExecutionMode, t.ReasoningEffort, strings.ReplaceAll(desc, "\n", " "))
	}
	return b.String()
}

// renderPlanAnswers formats caller-supplied answers for the ${PLAN_ANSWERS} prompt var.
func renderPlanAnswers(answers []types.PlanAnswer) string {
	if len(answers) == 0 {
		return "_No answers provided._"
	}
	var b strings.Builder
	for _, a := range answers {
		fmt.Fprintf(&b, "- Q %s: %s\n", a.QuestionID, a.Answer)
	}
	return b.String()
}
