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

// agentMCPExternalCmd is a token-authed MCP stdio bridge for ANY standalone
// MCP client (not spawned by nrflo, and not Claude-specific). It authenticates
// with a long-lived service token, exchanges it for a console session, and
// proxies tools/list + tools/call to that session's console tool routes over
// REST — holding no orchestrator or DB itself. The tool catalogue is entirely
// server-owned: whatever the server's console profile serves is what this
// bridge exposes, with zero per-tool code.
var agentMCPExternalCmd = &cobra.Command{
	Use:   "mcp-external",
	Short: "Token-authed JSON-RPC 2.0 MCP stdio bridge for standalone MCP clients",
	Long: `Bridge any standalone MCP client to a running nrflo_server over REST. The
bridge is a dumb transport: at startup it opens a console session and, for the
life of the process, forwards tools/list and tools/call to that session's
console tool routes. The tool set is whatever the server's console profile
serves — there is no tool list in the bridge itself.

Authentication is a long-lived nrflo service token (Settings → Administration →
Service Tokens); no agent session is involved. Configuration is via env:
  NRFLO_MCP_TOKEN  — service token, sent as Authorization: Bearer (required
                     unless a console session is adopted, below)
  NRFLO_PROJECT    — default project; optional, used when cwd auto-detect misses
  NRFLO_SERVER_URL — running server base URL (default http://127.0.0.1:6587)

When NRFLO_CONSOLE_TOKEN and NRFLO_CONSOLE_SESSION_ID are both set (injected by
'nrflo_server console'), the bridge adopts that pre-minted console session
instead of opening its own: no session is created or closed by the bridge, and
NRFLO_PROJECT pins the project with no cwd auto-detect. NRFLO_MCP_TOKEN is
still honored alongside an adopted session as the fallback for a 401
re-exchange (the adopted session was swept idle); without it, a 401 surfaces a
clear error instead of retrying with an empty bearer.

The project is resolved ONCE, at connect time (console sessions are
project-scoped): the working directory matched against project root paths →
NRFLO_PROJECT → the hidden global project.

Register with any MCP client, e.g. Claude Code:
  claude mcp add nrflo --env NRFLO_MCP_TOKEN=<token> --env NRFLO_PROJECT=<id> \
    -- nrflo_server agent mcp-external`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		token := os.Getenv("NRFLO_MCP_TOKEN")
		consoleToken := os.Getenv("NRFLO_CONSOLE_TOKEN")
		consoleSessionID := os.Getenv("NRFLO_CONSOLE_SESSION_ID")
		adopting := consoleToken != "" && consoleSessionID != ""
		if !adopting && token == "" {
			return fmt.Errorf("NRFLO_MCP_TOKEN must be set")
		}
		base := os.Getenv("NRFLO_SERVER_URL")
		if base == "" {
			base = "http://127.0.0.1:6587"
		}
		c := &nrfloHTTPClient{
			base:           strings.TrimRight(base, "/"),
			serviceToken:   token,
			defaultProject: os.Getenv("NRFLO_PROJECT"),
			// No client-side Timeout: long-running console tool calls can
			// block server-side for up to 25 minutes; cancellation comes from the
			// per-request ctx (runMCPStdioLoopWithCancel), not a client deadline.
			hc: &http.Client{},
		}
		if adopting {
			c.adoptConsoleSession(consoleSessionID, consoleToken, os.Getenv("NRFLO_PROJECT"))
		} else {
			openCtx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
			err := c.openConsoleSession(openCtx)
			cancel()
			if err != nil {
				return err
			}
		}
		defer c.closeConsoleSession() // no-op when the session was adopted, not owned
		return runMCPStdioLoopWithCancel(cmd.Context(), os.Stdin, os.Stdout, func(ctx context.Context, req mcpRequest) *mcpResponse {
			return dispatchExternalMCP(ctx, req, c)
		})
	},
}

// dispatchExternalMCP handles one JSON-RPC request for the external bridge.
// ctx is cancelled if the client cancels the call (or kills the bridge), so a
// blocking tool can stop its server-side run. tools/list and tools/call are
// pure HTTP passthroughs to the console tool routes — no tool name is known
// here or anywhere else in the bridge.
func dispatchExternalMCP(ctx context.Context, req mcpRequest, c *nrfloHTTPClient) *mcpResponse {
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
		tools, err := c.listConsoleTools(ctx)
		if err != nil {
			return makeMCPError(req.ID, -32603, err.Error())
		}
		specs := make([]map[string]interface{}, 0, len(tools))
		for _, t := range tools {
			schema := t.InputSchema
			if len(schema) == 0 {
				schema = json.RawMessage(`{"type":"object"}`)
			}
			specs = append(specs, map[string]interface{}{
				"name":        t.Name,
				"description": t.Description,
				"inputSchema": schema,
			})
		}
		return makeMCPResult(req.ID, map[string]interface{}{"tools": specs})
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
		output, isError, err := c.callConsoleTool(ctx, call.Name, call.Arguments)
		if err != nil {
			return mcpToolText(req.ID, err.Error(), true)
		}
		return mcpToolText(req.ID, output, isError)
	default:
		return makeMCPError(req.ID, -32601, fmt.Sprintf("method not found: %s", req.Method))
	}
}

// mcpToolText wraps text as an MCP tool_result content block.
func mcpToolText(id interface{}, text string, isError bool) *mcpResponse {
	return makeMCPResult(id, map[string]interface{}{
		"content": []map[string]interface{}{{"type": "text", "text": text}},
		"isError": isError,
	})
}
