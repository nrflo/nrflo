package cli

import (
	"bufio"
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
		caller := &clientSocketCaller{c: GetClient()}

		scanner := bufio.NewScanner(os.Stdin)
		writer := bufio.NewWriter(os.Stdout)

		emit := func(resp *mcpResponse) error {
			data, err := json.Marshal(resp)
			if err != nil {
				return err
			}
			if _, err := writer.Write(data); err != nil {
				return err
			}
			if err := writer.WriteByte('\n'); err != nil {
				return err
			}
			return writer.Flush()
		}

		for scanner.Scan() {
			line := scanner.Bytes()
			var req mcpRequest
			if err := json.Unmarshal(line, &req); err != nil {
				if werr := emit(makeMCPError(nil, -32700, "parse error: "+err.Error())); werr != nil {
					return werr
				}
				continue
			}
			resp := dispatchMCP(req, sessionID, instanceID, caller)
			if resp == nil {
				continue // notification — no reply
			}
			if werr := emit(resp); werr != nil {
				return werr
			}
		}
		return scanner.Err()
	},
}
