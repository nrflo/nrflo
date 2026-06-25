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
  NRFLO_PROJECT    — project scope, sent as X-Project (required)
  NRFLO_SERVER_URL — running server base URL (default http://127.0.0.1:6587)

Register with Claude Code:
  claude mcp add nrflo --env NRFLO_MCP_TOKEN=<token> --env NRFLO_PROJECT=<id> \
    -- nrflo_server agent mcp-external`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		token := os.Getenv("NRFLO_MCP_TOKEN")
		project := os.Getenv("NRFLO_PROJECT")
		if token == "" || project == "" {
			return fmt.Errorf("NRFLO_MCP_TOKEN and NRFLO_PROJECT must both be set")
		}
		base := os.Getenv("NRFLO_SERVER_URL")
		if base == "" {
			base = "http://127.0.0.1:6587"
		}
		c := &nrfloHTTPClient{
			base:    strings.TrimRight(base, "/"),
			token:   token,
			project: project,
			hc:      &http.Client{Timeout: 60 * time.Second},
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
		return makeMCPResult(req.ID, map[string]interface{}{"tools": externalToolSpecs(c.project)})
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
		}
		if err := json.Unmarshal(args, &in); err != nil {
			return "", fmt.Errorf("invalid arguments: %w", err)
		}
		if strings.TrimSpace(in.Question) == "" {
			return "", fmt.Errorf("question is required")
		}
		return c.deepResearch(ctx, in.Question)
	case "run_workflow":
		var in struct {
			Workflow     string `json:"workflow"`
			Instructions string `json:"instructions"`
		}
		if err := json.Unmarshal(args, &in); err != nil {
			return "", fmt.Errorf("invalid arguments: %w", err)
		}
		if strings.TrimSpace(in.Workflow) == "" {
			return "", fmt.Errorf("workflow is required")
		}
		id, err := c.runWorkflow(ctx, in.Workflow, in.Instructions)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("{\"instance_id\":%q}", id), nil
	case "get_workflow":
		var in struct {
			InstanceID string `json:"instance_id"`
		}
		if err := json.Unmarshal(args, &in); err != nil {
			return "", fmt.Errorf("invalid arguments: %w", err)
		}
		if strings.TrimSpace(in.InstanceID) == "" {
			return "", fmt.Errorf("instance_id is required")
		}
		state, err := c.getWorkflow(ctx, in.InstanceID)
		if err != nil {
			return "", err
		}
		b, err := json.MarshalIndent(state, "", "  ")
		if err != nil {
			return "", err
		}
		return string(b), nil
	case "list_workflows":
		raw, err := c.listWorkflows(ctx)
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
func externalToolSpecs(project string) []map[string]interface{} {
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
			"name":        "deep_research",
			"description": "Run nrflo's multi-source, fact-checked deep-research workflow for a question and return a cited markdown report. Blocks until the run finishes (can take several minutes — raise the MCP tool timeout, e.g. MCP_TIMEOUT). Runs in project '" + project + "'.",
			"inputSchema": obj(map[string]interface{}{"question": str("The research question or topic.")}, "question"),
		},
		{
			"name":        "run_workflow",
			"description": "Start a project-scoped nrflo workflow and return its instance_id immediately (non-blocking). Use get_workflow to poll status/results.",
			"inputSchema": obj(map[string]interface{}{
				"workflow":     str("Workflow name (e.g. 'deep-research', or a project workflow)."),
				"instructions": str("Optional instructions/prompt passed to the workflow."),
			}, "workflow"),
		},
		{
			"name":        "get_workflow",
			"description": "Fetch the current state (status, findings, agent history) of a workflow instance by id.",
			"inputSchema": obj(map[string]interface{}{"instance_id": str("Workflow instance id returned by run_workflow.")}, "instance_id"),
		},
		{
			"name":        "list_workflows",
			"description": "List the workflow definitions available in the project (includes global definitions like deep-research).",
			"inputSchema": obj(map[string]interface{}{}),
		},
	}
}
