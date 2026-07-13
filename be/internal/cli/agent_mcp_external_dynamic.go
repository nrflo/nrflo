package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// callDynamicWorkflowTool starts the bundled `dynamic` workflow and blocks
// (via pollPlanState) until the draft (or, in mode=auto, the run itself)
// reaches a status the caller must act on or a terminal status.
func callDynamicWorkflowTool(ctx context.Context, c *nrfloHTTPClient, args json.RawMessage) (string, error) {
	var in struct {
		Instructions string `json:"instructions"`
		Mode         string `json:"mode"`
		Project      string `json:"project"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	if strings.TrimSpace(in.Instructions) == "" {
		return "", fmt.Errorf("instructions is required")
	}
	project := c.resolveProject(ctx, in.Project)
	instanceID, err := c.startDynamicWorkflow(ctx, project, in.Instructions, in.Mode)
	if err != nil {
		return "", err
	}
	state, err := c.pollPlanState(ctx, project, instanceID)
	if err != nil {
		return "", err
	}
	c.attachPlanDraft(ctx, project, instanceID, state)
	b, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// callRevisePlanTool appends a new plan revision to a dynamic_workflow run,
// either from an edited manifest (plan) or planner feedback/answers.
func callRevisePlanTool(ctx context.Context, c *nrfloHTTPClient, args json.RawMessage) (string, error) {
	var in struct {
		InstanceID string           `json:"instance_id"`
		Revision   int              `json:"revision"`
		Plan       *json.RawMessage `json:"plan"`
		Feedback   string           `json:"feedback"`
		Answers    []map[string]any `json:"answers"`
		Project    string           `json:"project"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	if strings.TrimSpace(in.InstanceID) == "" {
		return "", fmt.Errorf("instance_id is required")
	}
	project := c.resolveProject(ctx, in.Project)
	body := map[string]any{"revision": in.Revision}
	if in.Plan != nil {
		body["manifest"] = *in.Plan
	}
	if in.Feedback != "" {
		body["feedback"] = in.Feedback
	}
	if len(in.Answers) > 0 {
		body["answers"] = in.Answers
	}
	rev, err := c.revisePlan(ctx, project, in.InstanceID, body)
	if err != nil {
		return "", err
	}
	b, err := json.MarshalIndent(rev, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// callApprovePlanTool approves a dynamic_workflow run's plan at the given
// revision (materializes it and resumes the run).
func callApprovePlanTool(ctx context.Context, c *nrfloHTTPClient, args json.RawMessage) (string, error) {
	var in struct {
		InstanceID string `json:"instance_id"`
		Revision   int    `json:"revision"`
		Project    string `json:"project"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	if strings.TrimSpace(in.InstanceID) == "" {
		return "", fmt.Errorf("instance_id is required")
	}
	project := c.resolveProject(ctx, in.Project)
	rev, err := c.approvePlan(ctx, project, in.InstanceID, in.Revision)
	if err != nil {
		return "", err
	}
	b, err := json.MarshalIndent(rev, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// dynamicWorkflowToolSpecs is the dynamic_workflow/revise_plan/approve_plan
// slice of externalToolSpecs' tool catalogue, split out to keep
// agent_mcp_external.go under the file size cap.
func dynamicWorkflowToolSpecs(projectArg map[string]interface{}) []map[string]interface{} {
	obj := func(props map[string]interface{}, required ...string) map[string]interface{} {
		s := map[string]interface{}{"type": "object", "properties": props}
		if len(required) > 0 {
			s["required"] = required
		}
		return s
	}
	str := func(desc string) map[string]interface{} {
		return map[string]interface{}{"type": "string", "description": desc}
	}
	return []map[string]interface{}{
		{
			"name":        "dynamic_workflow",
			"description": "Start the bundled, plan-driven `dynamic` workflow: a planner drafts a multi-agent manifest from your instructions, then (mode=\"approve\", default) the run parks waiting for you to drive it via revise_plan/approve_plan, or (mode=\"auto\", if enabled server-side) it auto-approves and runs to completion unattended. Blocks until the draft (or, in auto mode, the run) reaches a status you must act on or a terminal status, then returns the workflow state (including the plan block).",
			"inputSchema": obj(map[string]interface{}{
				"instructions": str("Goal / instructions for the planner to turn into a multi-agent plan."),
				"mode":         str("\"approve\" (default) parks for you to drive; \"auto\" auto-approves and runs unattended (requires the server's dynamic_workflow_auto_enabled setting)."),
				"project":      projectArg,
			}, "instructions"),
		},
		{
			"name":        "revise_plan",
			"description": "Revise a dynamic_workflow run's plan at the plan boundary: supply an edited manifest (plan) or feedback/answers to re-run the planner. revision must match the run's current plan revision (see get_workflow's plan.latest_revision) or the call is rejected as stale.",
			"inputSchema": obj(map[string]interface{}{
				"instance_id": str("Instance id returned by dynamic_workflow."),
				"revision":    map[string]interface{}{"type": "integer", "description": "Must match the run's current plan revision."},
				"plan":        map[string]interface{}{"type": "object", "description": "A full, edited plan manifest (version/goal/layers/questions) to store as-is after validation."},
				"feedback":    str("Feedback for the planner to re-run with (used when plan is omitted)."),
				"answers":     map[string]interface{}{"type": "array", "description": "Answers to open plan questions, [{question_id, answer}].", "items": map[string]interface{}{"type": "object"}},
				"project":     projectArg,
			}, "instance_id", "revision"),
		},
		{
			"name":        "approve_plan",
			"description": "Approve a dynamic_workflow run's plan at the given revision (materializes it and resumes the run). revision must match the run's current plan revision (see get_workflow's plan.latest_revision) or the call is rejected as stale.",
			"inputSchema": obj(map[string]interface{}{
				"instance_id": str("Instance id returned by dynamic_workflow."),
				"revision":    map[string]interface{}{"type": "integer", "description": "Must match the run's current plan revision."},
				"project":     projectArg,
			}, "instance_id", "revision"),
		},
	}
}
