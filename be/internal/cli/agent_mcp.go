package cli

import (
	"encoding/json"
	"os"

	"github.com/spf13/cobra"

	"be/internal/client"
)

// clientSocketCaller wraps client.Client to satisfy mcpSocketCaller.
type clientSocketCaller struct {
	c *client.Client
}

func (s *clientSocketCaller) Call(method string, params map[string]interface{}) (json.RawMessage, error) {
	var result json.RawMessage
	if err := s.c.ExecuteAndUnmarshal(method, params, &result); err != nil {
		return nil, err
	}
	return result, nil
}

var agentMCPCmd = &cobra.Command{
	Use:   "mcp",
	Short: "JSON-RPC 2.0 MCP stdio bridge for in-process API tool registry",
	Long: `Run a newline-delimited JSON-RPC 2.0 loop on stdin/stdout bridging MCP
tool list/call requests to the nrflo socket tool dispatcher.

Session and workflow instance IDs are read from environment variables:
  NRF_SESSION_ID           — agent session ID
  NRF_WORKFLOW_INSTANCE_ID — workflow instance ID

Used by Claude PTY agents spawned with --mcp-config pointing to this command.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionID := GetSessionID()
		instanceID := GetWorkflowInstanceID()
		observer := os.Getenv("NRF_OBSERVER") == "1"
		caller := &clientSocketCaller{c: GetClient()}

		return runMCPStdioLoop(os.Stdin, os.Stdout, func(req mcpRequest) *mcpResponse {
			return dispatchMCP(req, sessionID, instanceID, observer, caller)
		})
	},
}
