package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"be/internal/service"
)

// agentMCPExternalCmd is a token-authed MCP stdio proxy for STANDALONE Claude
// Code (not spawned by nrflo). Unlike `agent mcp` (session-bound, socket), it
// authenticates with a long-lived service token and proxies tool calls to a
// running `nrflo_server serve` over REST — holding no orchestrator or DB itself.
var agentMCPExternalCmd = &cobra.Command{
	Use:   "mcp-external",
	Short: "Token-authed JSON-RPC 2.0 MCP stdio proxy for standalone Claude Code clients",
	Long: `Bridge a standalone Claude Code session to a running nrflo_server over REST,
exposing deep_research / run_workflow / get_workflow / list_workflows as MCP tools.

Authentication is a long-lived nrflo service token (Settings → Administration →
Service Tokens); no agent session is involved. Configuration is via env:
  NRFLO_MCP_TOKEN  — service token, sent as Authorization: Bearer (required)
  NRFLO_PROJECT    — default project (X-Project); optional
  NRFLO_SERVER_URL — running server base URL (default http://127.0.0.1:6587)

A global-scope token works across projects; a project-scope token is limited to
its own project. Per call the project resolves as: explicit 'project' arg → the
working directory matched against project root paths → NRFLO_PROJECT → the
hidden global project (so project-agnostic tools like deep_research work from
any directory with no config).

Register with Claude Code:
  claude mcp add nrflo --env NRFLO_MCP_TOKEN=<token> --env NRFLO_PROJECT=<id> \
    -- nrflo_server agent mcp-external`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		token := os.Getenv("NRFLO_MCP_TOKEN")
		if token == "" {
			return fmt.Errorf("NRFLO_MCP_TOKEN must be set")
		}
		base := os.Getenv("NRFLO_SERVER_URL")
		if base == "" {
			base = "http://127.0.0.1:6587"
		}
		c := &nrfloHTTPClient{
			base:           strings.TrimRight(base, "/"),
			token:          token,
			defaultProject: os.Getenv("NRFLO_PROJECT"), // optional default; per-call `project` overrides
			hc:             &http.Client{Timeout: 60 * time.Second},
		}
		return runMCPStdioLoop(os.Stdin, os.Stdout, func(req mcpRequest) *mcpResponse {
			return dispatchExternalMCP(req, c)
		})
	},
}

// dispatchExternalMCP handles one JSON-RPC request for the external proxy.
func dispatchExternalMCP(req mcpRequest, c *nrfloHTTPClient) *mcpResponse {
	switch req.Method {
	case "initialize":
		return makeMCPResult(req.ID, map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]interface{}{"tools": map[string]interface{}{}},
			"serverInfo":      map[string]interface{}{"name": "nrflo-external", "version": "1.0"},
		})
	case "notifications/initialized":
		return nil
	case "tools/list":
		return makeMCPResult(req.ID, map[string]interface{}{"tools": externalToolSpecs(c.defaultProject)})
	case "tools/call":
		var call struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if req.Params != nil {
			if err := json.Unmarshal(req.Params, &call); err != nil {
				return makeMCPError(req.ID, -32602, fmt.Sprintf("invalid params: %v", err))
			}
		}
		if call.Name == "" {
			return makeMCPError(req.ID, -32602, "name is required")
		}
		text, err := callExternalTool(c, call.Name, call.Arguments)
		if err != nil {
			return mcpToolText(req.ID, err.Error(), true)
		}
		return mcpToolText(req.ID, text, false)
	default:
		return makeMCPError(req.ID, -32601, fmt.Sprintf("method not found: %s", req.Method))
	}
}

// resolveProject picks the project for a tool call, in order: explicit arg →
// cwd auto-detect (the proxy's working dir matched against project root_paths) →
// NRFLO_PROJECT default → the hidden global project. It never errors: execution
// always needs a concrete project, and project-agnostic tools (deep_research)
// run in the global project when nothing else resolves. A project-specific
// workflow run there simply 404s server-side ("workflow not found").
func (c *nrfloHTTPClient) resolveProject(ctx context.Context, arg string) string {
	if p := strings.TrimSpace(arg); p != "" {
		return p
	}
	if p := c.cwdProject(ctx); p != "" {
		return p
	}
	if c.defaultProject != "" {
		return c.defaultProject
	}
	return service.GlobalProjectID
}

// callExternalTool routes a tool call to the REST client and returns its text.
func callExternalTool(c *nrfloHTTPClient, name string, args json.RawMessage) (string, error) {
	ctx := context.Background()
	if len(args) == 0 {
		args = json.RawMessage("{}")
	}
	switch name {
	case "deep_research":
		var in struct {
			Question string `json:"question"`
			Context  string `json:"context"`
			Project  string `json:"project"`
		}
		if err := json.Unmarshal(args, &in); err != nil {
			return "", fmt.Errorf("invalid arguments: %w", err)
		}
		if strings.TrimSpace(in.Question) == "" {
			return "", fmt.Errorf("question is required")
		}
		project := c.resolveProject(ctx, in.Project)
		return c.deepResearch(ctx, project, in.Question, in.Context)
	case "run_workflow":
		var in struct {
			Workflow     string `json:"workflow"`
			Instructions string `json:"instructions"`
			Project      string `json:"project"`
		}
		if err := json.Unmarshal(args, &in); err != nil {
			return "", fmt.Errorf("invalid arguments: %w", err)
		}
		if strings.TrimSpace(in.Workflow) == "" {
			return "", fmt.Errorf("workflow is required")
		}
		project := c.resolveProject(ctx, in.Project)
		id, err := c.runWorkflow(ctx, project, in.Workflow, in.Instructions, "")
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("{\"instance_id\":%q}", id), nil
	case "get_workflow":
		var in struct {
			InstanceID string `json:"instance_id"`
			Project    string `json:"project"`
		}
		if err := json.Unmarshal(args, &in); err != nil {
			return "", fmt.Errorf("invalid arguments: %w", err)
		}
		if strings.TrimSpace(in.InstanceID) == "" {
			return "", fmt.Errorf("instance_id is required")
		}
		project := c.resolveProject(ctx, in.Project)
		state, err := c.getWorkflow(ctx, project, in.InstanceID)
		if err != nil {
			return "", err
		}
		b, err := json.MarshalIndent(state, "", "  ")
		if err != nil {
			return "", err
		}
		return string(b), nil
	case "list_workflows":
		var in struct {
			Project string `json:"project"`
		}
		if err := json.Unmarshal(args, &in); err != nil {
			return "", fmt.Errorf("invalid arguments: %w", err)
		}
		project := c.resolveProject(ctx, in.Project)
		raw, err := c.listWorkflows(ctx, project)
		if err != nil {
			return "", err
		}
		return string(raw), nil
	default:
		return "", fmt.Errorf("unknown tool: %s", name)
	}
}

// mcpToolText wraps text as an MCP tool_result content block.
func mcpToolText(id interface{}, text string, isError bool) *mcpResponse {
	return makeMCPResult(id, map[string]interface{}{
		"content": []map[string]interface{}{{"type": "text", "text": text}},
		"isError": isError,
	})
}

// externalToolSpecs is the static MCP tool catalogue for the external proxy.
// defaultProject (NRFLO_PROJECT) is woven into the `project` arg description so
// the model knows whether it must pass one.
func externalToolSpecs(defaultProject string) []map[string]interface{} {
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
	projDesc := "Project to run in. Optional — if omitted it auto-detects from the working directory (matched against project root paths), then NRFLO_PROJECT, then the hidden global project."
	if defaultProject != "" {
		projDesc += " NRFLO_PROJECT default: '" + defaultProject + "'."
	}
	projectArg := str(projDesc)
	return []map[string]interface{}{
		{
			"name":        "deep_research",
			"description": "Run nrflo's multi-source, fact-checked deep-research workflow for a question and return a cited markdown report. Blocks until the run finishes (can take several minutes — raise the MCP tool timeout, e.g. MCP_TIMEOUT). Pass `context` to ground the research in your current project and conversation.",
			"inputSchema": obj(map[string]interface{}{
				"question": str("The research question or topic."),
				"context":  str("Optional. Relevant context from your current conversation and project to focus the research — e.g. what the user is building, the tech stack and versions in use, constraints, and decisions or facts already established. Summarize it concisely in a few sentences. Leave empty for general, project-agnostic web research."),
				"project":  projectArg,
			}, "question"),
		},
		{
			"name":        "run_workflow",
			"description": "Start a project-scoped nrflo workflow and return its instance_id immediately (non-blocking). Use get_workflow to poll status/results.",
			"inputSchema": obj(map[string]interface{}{
				"workflow":     str("Workflow name (e.g. 'deep-research', or a project workflow)."),
				"instructions": str("Optional instructions/prompt passed to the workflow."),
				"project":      projectArg,
			}, "workflow"),
		},
		{
			"name":        "get_workflow",
			"description": "Fetch the current state (status, findings, agent history) of a workflow instance by id.",
			"inputSchema": obj(map[string]interface{}{"instance_id": str("Workflow instance id returned by run_workflow."), "project": projectArg}, "instance_id"),
		},
		{
			"name":        "list_workflows",
			"description": "List the workflow definitions available in a project (includes global definitions like deep-research).",
			"inputSchema": obj(map[string]interface{}{"project": projectArg}),
		},
	}
}
