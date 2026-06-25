package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// nrfloHTTPClient is a thin REST client into a running `nrflo_server serve`,
// authenticated with a long-lived service token. The target project is supplied
// per request (so one global token can drive many projects); defaultProject is
// used when a tool call omits it. The external MCP proxy holds no DB pool or
// orchestrator — every tool call is a REST request into the live server.
type nrfloHTTPClient struct {
	base           string // e.g. http://127.0.0.1:6587
	token          string // service token (Authorization: Bearer)
	defaultProject string // X-Project used when a tool call omits `project`
	hc             *http.Client

	// cwd→project auto-detect, resolved lazily and cached for the process
	// lifetime — but only after a successful lookup (see cwdProject).
	cwdResolved  bool
	cwdProjectID string
}

// deepResearch polling cadence; vars so tests can shrink them.
var (
	deepResearchPollInterval = 3 * time.Second
	deepResearchMaxWait      = 25 * time.Minute
)

func (c *nrfloHTTPClient) do(ctx context.Context, project, method, path string, body any, out any) error {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, rdr)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("X-Project", project)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close() //nolint:errcheck
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("nrflo %s %s -> %d: %s", method, path, resp.StatusCode, strings.TrimSpace(string(data)))
	}
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("decode %s response: %w", path, err)
		}
	}
	return nil
}

// runWorkflow starts a project-scoped workflow and returns its instance_id.
func (c *nrfloHTTPClient) runWorkflow(ctx context.Context, project, workflow, instructions string) (string, error) {
	var res struct {
		InstanceID string `json:"instance_id"`
	}
	err := c.do(ctx, project, http.MethodPost,
		"/api/v1/projects/"+url.PathEscape(project)+"/workflow/run",
		map[string]any{"workflow": workflow, "instructions": instructions}, &res)
	if err != nil {
		return "", err
	}
	if res.InstanceID == "" {
		return "", fmt.Errorf("run %q: server returned no instance_id", workflow)
	}
	return res.InstanceID, nil
}

// getWorkflow returns the v4 state map for one workflow instance.
func (c *nrfloHTTPClient) getWorkflow(ctx context.Context, project, instanceID string) (map[string]any, error) {
	var res struct {
		State map[string]any `json:"state"`
	}
	err := c.do(ctx, project, http.MethodGet,
		"/api/v1/projects/"+url.PathEscape(project)+"/workflow?instance_id="+url.QueryEscape(instanceID),
		nil, &res)
	if err != nil {
		return nil, err
	}
	return res.State, nil
}

// listWorkflows returns the project's selectable workflow definitions (includes
// global definitions like deep-research).
func (c *nrfloHTTPClient) listWorkflows(ctx context.Context, project string) (json.RawMessage, error) {
	var raw json.RawMessage
	if err := c.do(ctx, project, http.MethodGet, "/api/v1/workflows", nil, &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

// deepResearch runs the deep-research workflow and blocks until it finishes,
// returning the synthesized `report` finding. Polling is client-side (many short
// GETs) rather than one long request, so no single HTTP call runs for minutes.
func (c *nrfloHTTPClient) deepResearch(ctx context.Context, project, question string) (string, error) {
	instanceID, err := c.runWorkflow(ctx, project, "deep-research", question)
	if err != nil {
		return "", err
	}
	deadline := time.Now().Add(deepResearchMaxWait)
	for {
		state, err := c.getWorkflow(ctx, project, instanceID)
		if err != nil {
			return "", err
		}
		switch fmt.Sprint(state["status"]) {
		case "completed", "project_completed":
			return extractReport(state, instanceID)
		case "failed":
			return "", fmt.Errorf("deep-research run %s failed; inspect it with get_workflow(instance_id=%s)", instanceID, instanceID)
		}
		if time.Now().After(deadline) {
			return "", fmt.Errorf("deep-research run %s still running after %s; poll get_workflow with instance_id=%s",
				instanceID, deepResearchMaxWait, instanceID)
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(deepResearchPollInterval):
		}
	}
}

// extractReport pulls the `report` finding out of a terminal v4 state.
func extractReport(state map[string]any, instanceID string) (string, error) {
	wf, _ := state["workflow_findings"].(map[string]any)
	report, ok := wf["report"]
	if !ok {
		return "", fmt.Errorf("deep-research run %s completed but emitted no 'report' finding", instanceID)
	}
	if s, isStr := report.(string); isStr {
		return s, nil
	}
	b, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}
