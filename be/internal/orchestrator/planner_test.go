package orchestrator

import (
	"context"
	"strings"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/service"
	"be/internal/types"
)

// insertLocalPlannerDef inserts a workflow-local agent_definitions row with
// node_role='planner' under (env.project, workflowID).
func insertLocalPlannerDef(t *testing.T, env *testEnv, workflowID, id, model, executionMode string) {
	t.Helper()
	now := clock.Real().Now().UTC().Format(time.RFC3339Nano)
	if _, err := env.pool.Exec(
		`INSERT INTO agent_definitions (id, project_id, workflow_id, model, timeout, execution_mode, node_role, created_at, updated_at)
		 VALUES (?, ?, ?, ?, 45, ?, 'planner', ?, ?)`,
		id, env.project, workflowID, model, executionMode, now, now,
	); err != nil {
		t.Fatalf("insert local planner def: %v", err)
	}
}

// TestResolvePlannerDef_WorkflowLocalOverride verifies that a workflow-local
// agent_definitions row with node_role='planner' takes precedence over the
// system default.
func TestResolvePlannerDef_WorkflowLocalOverride(t *testing.T) {
	env := newTestEnv(t)
	insertLocalPlannerDef(t, env, "test", "local-planner", "opus", "api")

	cfg, err := env.orch.resolvePlannerDef(env.pool, env.project, "test")
	if err != nil {
		t.Fatalf("resolvePlannerDef() error: %v", err)
	}
	if cfg.ID != "local-planner" {
		t.Errorf("ID = %q, want %q", cfg.ID, "local-planner")
	}
	if cfg.ExecutionMode != "api" {
		t.Errorf("ExecutionMode = %q, want %q", cfg.ExecutionMode, "api")
	}
	if cfg.Model != "opus" {
		t.Errorf("Model = %q, want %q", cfg.Model, "opus")
	}
}

// TestResolvePlannerDef_SystemDefaultFallback verifies that, absent a
// workflow-local planner def, resolvePlannerDef falls back to the
// system_agent_definitions role='planner' row for the cli_interactive
// backend (seeded by migration 000158 as "planner-system") ahead of the
// "planner-system-api" row.
func TestResolvePlannerDef_SystemDefaultFallback(t *testing.T) {
	env := newTestEnv(t)

	cfg, err := env.orch.resolvePlannerDef(env.pool, env.project, "test")
	if err != nil {
		t.Fatalf("resolvePlannerDef() error: %v", err)
	}
	if cfg.ID != "planner-system" {
		t.Errorf("ID = %q, want %q", cfg.ID, "planner-system")
	}
	if cfg.ExecutionMode != "cli_interactive" {
		t.Errorf("ExecutionMode = %q, want %q", cfg.ExecutionMode, "cli_interactive")
	}
}

// TestResolvePlannerDef_ReasoningEffort_WorkflowLocalOverride verifies a
// workflow-local planner def's reasoning_effort override threads through
// resolvePlannerDef into plannerAgentConfig, and that omitting the column
// (system-default fallback) resolves to nil.
func TestResolvePlannerDef_ReasoningEffort_WorkflowLocalOverride(t *testing.T) {
	env := newTestEnv(t)
	insertLocalPlannerDef(t, env, "test", "local-planner-effort", "opus_4_8", "cli_interactive")
	if _, err := env.pool.Exec(
		`UPDATE agent_definitions SET reasoning_effort = 'xhigh' WHERE id = 'local-planner-effort'`,
	); err != nil {
		t.Fatalf("set reasoning_effort: %v", err)
	}

	cfg, err := env.orch.resolvePlannerDef(env.pool, env.project, "test")
	if err != nil {
		t.Fatalf("resolvePlannerDef() error: %v", err)
	}
	if cfg.ReasoningEffort == nil || *cfg.ReasoningEffort != "xhigh" {
		t.Errorf("ReasoningEffort = %v, want xhigh", cfg.ReasoningEffort)
	}
}

// TestResolvePlannerDef_ReasoningEffort_SystemDefaultFallbackIsNil verifies
// that absent a workflow-local override, the system planner def's
// (unset) reasoning_effort resolves to nil rather than a zero-value "".
func TestResolvePlannerDef_ReasoningEffort_SystemDefaultFallbackIsNil(t *testing.T) {
	env := newTestEnv(t)

	cfg, err := env.orch.resolvePlannerDef(env.pool, env.project, "test")
	if err != nil {
		t.Fatalf("resolvePlannerDef() error: %v", err)
	}
	if cfg.ReasoningEffort != nil {
		t.Errorf("ReasoningEffort = %v, want nil (system planner-system def has no override)", *cfg.ReasoningEffort)
	}
}

// TestRenderTemplateLibrary_Empty verifies the placeholder string returned
// when no templates are configured.
func TestRenderTemplateLibrary_Empty(t *testing.T) {
	got := renderTemplateLibrary(nil)
	want := "_No templates configured for this workflow — the plan cannot include any nodes._"
	if got != want {
		t.Errorf("renderTemplateLibrary(nil) = %q, want %q", got, want)
	}
}

// TestRenderTemplateLibrary_NonEmpty verifies each template's id/model/
// execution_mode/description appear in the rendered output verbatim (the
// description, not the prompt body, is the planner's selection surface —
// see planner.go renderTemplateLibrary), and that a missing description
// falls back to a placeholder instead of an empty line.
func TestRenderTemplateLibrary_NonEmpty(t *testing.T) {
	templates := []service.PlanTemplate{
		{ID: "tpl-a", Model: "sonnet", ExecutionMode: "api", ReasoningEffort: "high", Prompt: "short prompt", Description: "Reviews code for correctness."},
		{ID: "tpl-b", Model: "opus", ExecutionMode: "cli_interactive", Prompt: "irrelevant prompt body"},
	}

	got := renderTemplateLibrary(templates)

	for _, want := range []string{
		"tpl-a", "sonnet", "api", "effort=high", "Reviews code for correctness.",
		"tpl-b", "opus", "cli_interactive", "effort=", "(no description provided)",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("renderTemplateLibrary() missing %q in output:\n%s", want, got)
		}
	}
	if strings.Contains(got, "irrelevant prompt body") {
		t.Error("renderTemplateLibrary() leaked the prompt body — description is the selection surface, not the prompt")
	}
}

// TestRenderTemplateLibrary_EmptyEffortRendersBlank verifies a template with
// no effective reasoning effort (EnabledTemplates left it "") still renders
// the "effort=" marker with a blank value, rather than omitting the field.
func TestRenderTemplateLibrary_EmptyEffortRendersBlank(t *testing.T) {
	templates := []service.PlanTemplate{
		{ID: "tpl-noeffort", Model: "sonnet", ExecutionMode: "cli_interactive"},
	}
	got := renderTemplateLibrary(templates)
	if !strings.Contains(got, "tpl-noeffort (sonnet, cli_interactive, effort=)") {
		t.Errorf("renderTemplateLibrary() = %q, want a line with a blank effort=", got)
	}
}

// TestRenderTemplateLibrary_DescriptionNewlinesCollapsed verifies a
// multi-line description is rendered on a single line per template so the
// library stays one entry per line for the planner to scan.
func TestRenderTemplateLibrary_DescriptionNewlinesCollapsed(t *testing.T) {
	templates := []service.PlanTemplate{
		{ID: "tpl-multiline", Model: "sonnet", ExecutionMode: "cli_interactive", Description: "line one\nline two"},
	}
	got := renderTemplateLibrary(templates)
	if !strings.Contains(got, "line one line two") {
		t.Errorf("renderTemplateLibrary() did not collapse newlines in description:\n%s", got)
	}
	if strings.Contains(got, "line one\nline two") {
		t.Error("renderTemplateLibrary() left an embedded newline inside a template's description block")
	}
}

// TestRenderPlanAnswers_Empty verifies the placeholder string returned when
// no answers are supplied.
func TestRenderPlanAnswers_Empty(t *testing.T) {
	got := renderPlanAnswers(nil)
	want := "_No answers provided._"
	if got != want {
		t.Errorf("renderPlanAnswers(nil) = %q, want %q", got, want)
	}
}

// TestRenderPlanAnswers_NonEmpty verifies each answer's QuestionID/Answer
// pair appears in the rendered output.
func TestRenderPlanAnswers_NonEmpty(t *testing.T) {
	answers := []types.PlanAnswer{
		{QuestionID: "q1", Answer: "yes, use approach A"},
		{QuestionID: "q2", Answer: "no caching needed"},
	}

	got := renderPlanAnswers(answers)

	for _, want := range []string{"q1", "yes, use approach A", "q2", "no caching needed"} {
		if !strings.Contains(got, want) {
			t.Errorf("renderPlanAnswers() missing %q in output:\n%s", want, got)
		}
	}
}

// TestPlannerNodeID_ReservedConstant is a low-priority sanity check that the
// reserved node id constant matches what the rest of the codebase (e.g.
// service/workflow_response.go's transientAgentTypeExclusion) expects.
func TestPlannerNodeID_ReservedConstant(t *testing.T) {
	if plannerNodeID != "_planner" {
		t.Errorf("plannerNodeID = %q, want %q", plannerNodeID, "_planner")
	}
}

// TestRunPlanner_UnknownInstance verifies RunPlanner fails fast (before any
// spawn/network activity) when instanceID does not resolve to a workflow
// instance.
func TestRunPlanner_UnknownInstance(t *testing.T) {
	env := newTestEnv(t)

	_, err := env.orch.RunPlanner(context.Background(), "nonexistent-instance-xyz", service.PlannerInput{Goal: "do the thing"})
	if err == nil {
		t.Fatal("RunPlanner() = nil error; want error for unknown instance")
	}
	if !strings.Contains(err.Error(), "resolve workflow instance") {
		t.Errorf("error = %q, want contains %q", err.Error(), "resolve workflow instance")
	}
}

// TestRunPlanner_NoRootPath verifies RunPlanner fails fast when the
// instance's project has no root_path configured, before any spawn/network
// activity.
func TestRunPlanner_NoRootPath(t *testing.T) {
	env := newTestEnv(t)

	const ticketID = "planner-no-root-001"
	env.createTicket(t, ticketID, "Planner No Root Test")
	instanceID := env.initWorkflow(t, ticketID)

	if _, err := env.pool.Exec(`UPDATE projects SET root_path = NULL WHERE id = ?`, env.project); err != nil {
		t.Fatalf("clear project root_path: %v", err)
	}

	_, err := env.orch.RunPlanner(context.Background(), instanceID, service.PlannerInput{Goal: "do the thing"})
	if err == nil {
		t.Fatal("RunPlanner() = nil error; want error for missing root_path")
	}
	if !strings.Contains(err.Error(), "root_path") {
		t.Errorf("error = %q, want contains %q", err.Error(), "root_path")
	}
}
