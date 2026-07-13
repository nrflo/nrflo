package service

import (
	"strings"
	"testing"
)

func TestValidatePlanManifest_UnknownTemplate(t *testing.T) {
	t.Parallel()
	pool, projectID, workflowID := setupPlanValidateEnv(t)

	m := baseValidManifest("worker")
	m.Layers[0].Nodes[0].Template = "nope-does-not-exist"
	m.Layers[1].Nodes[0].Template = "worker" // keep the final layer's template valid to isolate the violation

	err := ValidatePlanManifest(pool, projectID, workflowID, m)
	if err == nil {
		t.Fatal("expected error for unknown template, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, `unknown template "nope-does-not-exist"`) {
		t.Fatalf("expected error to name the unknown template id, got: %v", msg)
	}
	if !strings.Contains(msg, "available templates:") || !strings.Contains(msg, "worker") {
		t.Fatalf("expected error to list available templates including 'worker', got: %v", msg)
	}
}

// A template whose model is not currently enabled must be rejected, naming
// the template, execution mode, and model so the planner can replan.
func TestValidatePlanManifest_TemplateModelDisabled(t *testing.T) {
	t.Parallel()
	pool, projectID, workflowID := setupPlanValidateEnv(t)

	insertFanoutTemplate(t, pool, projectID, workflowID, "disabled-cli", "sonnet", "cli_interactive")
	if _, err := pool.Exec(`UPDATE cli_models SET enabled = 0 WHERE id = 'sonnet'`); err != nil {
		t.Fatalf("disable cli model: %v", err)
	}

	m := baseValidManifest("disabled-cli")
	err := ValidatePlanManifest(pool, projectID, workflowID, m)
	if err == nil {
		t.Fatal("expected error for disabled cli model, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, `"disabled-cli"`) {
		t.Fatalf("expected error to name the template id, got: %v", msg)
	}
	if !strings.Contains(msg, "cli_interactive") {
		t.Fatalf("expected error to name the execution mode, got: %v", msg)
	}
	if !strings.Contains(msg, `"sonnet"`) {
		t.Fatalf("expected error to name the model id, got: %v", msg)
	}
}

// Same as above but for an api-mode template whose api_models row is disabled.
func TestValidatePlanManifest_APITemplateModelDisabled(t *testing.T) {
	t.Parallel()
	pool, projectID, workflowID := setupPlanValidateEnv(t)

	insertFanoutTemplate(t, pool, projectID, workflowID, "disabled-api", "sonnet", "api")
	if _, err := pool.Exec(`UPDATE api_models SET enabled = 0 WHERE id = 'sonnet'`); err != nil {
		t.Fatalf("disable api model: %v", err)
	}

	m := baseValidManifest("disabled-api")
	err := ValidatePlanManifest(pool, projectID, workflowID, m)
	if err == nil {
		t.Fatal("expected error for disabled api model, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, `"disabled-api"`) {
		t.Fatalf("expected error to name the template id, got: %v", msg)
	}
	if !strings.Contains(msg, "api") {
		t.Fatalf("expected error to name the execution mode, got: %v", msg)
	}
	if !strings.Contains(msg, `"sonnet"`) {
		t.Fatalf("expected error to name the model id, got: %v", msg)
	}
}

// A planner-role definition (node_role='planner') is never a fanout_template,
// so AllowedTemplates excludes it; referencing its id as a node's template
// simply hits the "unknown template" path — there is no dedicated
// self-reference guard in the validator.
func TestValidatePlanManifest_PlannerDefReferencedAsTemplate_IsUnknownTemplate(t *testing.T) {
	t.Parallel()
	pool, projectID, workflowID := setupPlanValidateEnv(t)

	insertPlannerDef(t, pool, projectID, workflowID, "the-planner")

	m := baseValidManifest("worker")
	m.Layers[0].Nodes[0].Template = "the-planner"

	err := ValidatePlanManifest(pool, projectID, workflowID, m)
	if err == nil {
		t.Fatal("expected error referencing a planner def as a template, got nil")
	}
	if !strings.Contains(err.Error(), `unknown template "the-planner"`) {
		t.Fatalf("expected 'unknown template' error naming the-planner, got: %v", err)
	}
}
