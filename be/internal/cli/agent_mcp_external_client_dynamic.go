package cli

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"be/internal/model"
)

// dynamicWorkflow polling cadence (pollPlanState); vars so tests can shrink them.
var (
	dynamicWorkflowPollInterval = 3 * time.Second
	dynamicWorkflowMaxWait      = 25 * time.Minute
)

// startDynamicWorkflow starts the bundled plan-driven `dynamic` workflow and
// returns its instance_id — POST /projects/{id}/dynamic-workflow.
func (c *nrfloHTTPClient) startDynamicWorkflow(ctx context.Context, project, instructions, mode string) (string, error) {
	var res struct {
		InstanceID string `json:"instance_id"`
	}
	body := map[string]any{"instructions": instructions}
	if mode != "" {
		body["mode"] = mode
	}
	err := c.do(ctx, project, http.MethodPost,
		"/api/v1/projects/"+url.PathEscape(project)+"/dynamic-workflow",
		body, &res)
	if err != nil {
		return "", err
	}
	if res.InstanceID == "" {
		return "", fmt.Errorf("dynamic_workflow: server returned no instance_id")
	}
	return res.InstanceID, nil
}

// getPlan returns the plan draft (head/manifest/questions/templates) for a
// workflow instance — GET /workflow-instances/{iid}/plan.
func (c *nrfloHTTPClient) getPlan(ctx context.Context, project, instanceID string) (map[string]any, error) {
	var raw map[string]any
	if err := c.do(ctx, project, http.MethodGet,
		"/api/v1/workflow-instances/"+url.PathEscape(instanceID)+"/plan",
		nil, &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

// attachPlanDraft folds the full plan draft into a run parked at the plan
// boundary. get_workflow's derived plan block carries only the revision
// numbers and open questions, but a caller asked to revise or approve must see
// the manifest itself — so merge manifest/questions/templates in, keeping
// latest_revision (which revise_plan/approve_plan must be pinned to).
// Best-effort: a plan the server won't hand back leaves the state untouched.
func (c *nrfloHTTPClient) attachPlanDraft(ctx context.Context, project, instanceID string, state map[string]any) {
	if !model.IsPlanSuspended(model.WorkflowInstanceStatus(fmt.Sprint(state["status"]))) {
		return
	}
	draft, err := c.getPlan(ctx, project, instanceID)
	if err != nil {
		return
	}
	block, ok := state["plan"].(map[string]any)
	if !ok {
		state["plan"] = draft
		return
	}
	for _, key := range []string{"manifest", "questions", "templates"} {
		if v, ok := draft[key]; ok {
			block[key] = v
		}
	}
}

// revisePlan appends a new plan revision to instanceID's plan — POST .../plan/revise.
func (c *nrfloHTTPClient) revisePlan(ctx context.Context, project, instanceID string, body map[string]any) (map[string]any, error) {
	var raw map[string]any
	if err := c.do(ctx, project, http.MethodPost,
		"/api/v1/workflow-instances/"+url.PathEscape(instanceID)+"/plan/revise",
		body, &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

// approvePlan approves instanceID's plan at revision — POST .../plan/approve.
func (c *nrfloHTTPClient) approvePlan(ctx context.Context, project, instanceID string, revision int) (map[string]any, error) {
	var raw map[string]any
	if err := c.do(ctx, project, http.MethodPost,
		"/api/v1/workflow-instances/"+url.PathEscape(instanceID)+"/plan/approve",
		map[string]any{"revision": revision}, &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

// pollPlanState polls GET /projects/{id}/workflow?instance_id= (client-side,
// many short GETs — mirrors deepResearch's loop) until instanceID's status
// leaves running/planning (a plan-boundary or terminal status) or
// dynamicWorkflowMaxWait elapses; a cancelled/timed-out wait best-effort stops
// the server-side run so it does not keep executing (and billing) unattended.
func (c *nrfloHTTPClient) pollPlanState(ctx context.Context, project, instanceID string) (map[string]any, error) {
	deadline := time.Now().Add(dynamicWorkflowMaxWait)
	for {
		state, err := c.getWorkflow(ctx, project, instanceID)
		if err != nil {
			if ctx.Err() != nil {
				c.stopWorkflow(project, instanceID)
				return nil, ctx.Err()
			}
			return nil, err
		}
		switch fmt.Sprint(state["status"]) {
		case "running", "planning":
			// keep polling
		default:
			return state, nil
		}
		if time.Now().After(deadline) {
			return state, fmt.Errorf("dynamic workflow %s still %s after %s; poll get_workflow with instance_id=%s",
				instanceID, fmt.Sprint(state["status"]), dynamicWorkflowMaxWait, instanceID)
		}
		select {
		case <-ctx.Done():
			c.stopWorkflow(project, instanceID)
			return nil, ctx.Err()
		case <-time.After(dynamicWorkflowPollInterval):
		}
	}
}
