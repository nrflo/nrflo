package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"
)

// These commands are agent infrastructure invoked by the spawner / Claude hooks
// (not by the agent itself). They are registered on nrflo_server's agentInfraCmd
// in agent_infra.go and connect back to the running server over the Unix socket.

// context-update flag.
var agentContextUpdatePctUsed float64

var agentContextUpdateCmd = &cobra.Command{
	Use:   "context-update <session-id>",
	Short: "Update context usage for an agent session (used by hooks)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// No RequireProject — session_id is globally unique
		if err := CheckServer(); err != nil {
			// Server not running — exit silently
			return nil
		}

		sessionID := args[0]
		contextLeft := int(100 - agentContextUpdatePctUsed)
		if contextLeft < 0 {
			contextLeft = 0
		}
		if contextLeft > 100 {
			contextLeft = 100
		}

		c := GetClient()
		reqParams := map[string]interface{}{
			"session_id":   sessionID,
			"context_left": contextLeft,
		}

		return c.ExecuteAndUnmarshal("agent.context_update", reqParams, nil)
	},
}

var agentRecordEventCmd = &cobra.Command{
	Use:   "record-event",
	Short: "Forward a Claude hook event to the server (used by --settings hooks)",
	Long: `Read a Claude hook JSON payload from stdin and forward it to the server
via the Unix socket. Used automatically by Claude --settings PreToolUse/PostToolUse
hooks. Exits 0 on success, 1 on error. Silently exits 0 when the server is not running
(hooks must not block the agent).

Context is read from environment variables set by the spawner:
  NRF_SESSION_ID          — current agent session ID (required)
  NRF_WORKFLOW_INSTANCE_ID — workflow instance ID`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if !GetClient().IsServerRunning() {
			return nil // server not running — exit silently so hooks don't block the agent
		}

		sessionID := GetSessionID()
		if sessionID == "" {
			return nil // no session — hook fired outside of spawner context, ignore
		}

		raw, err := io.ReadAll(cmd.InOrStdin())
		if err != nil {
			return fmt.Errorf("failed to read stdin: %w", err)
		}

		// Validate that hook_event_name is present
		var probe map[string]interface{}
		if err := json.Unmarshal(raw, &probe); err != nil {
			return fmt.Errorf("invalid hook JSON: %w", err)
		}
		if _, ok := probe["hook_event_name"]; !ok {
			return fmt.Errorf("hook JSON missing hook_event_name field")
		}

		reqParams := map[string]interface{}{
			"event": json.RawMessage(raw),
		}
		addSpawnerIDs(reqParams)

		// Enforce a 2s hard deadline — hooks must not block the agent.
		var resp struct {
			StopDecision *struct {
				Block  bool   `json:"block"`
				Reason string `json:"reason"`
			} `json:"stop_decision"`
		}
		type result struct{ err error }
		ch := make(chan result, 1)
		go func() {
			ch <- result{err: GetClient().ExecuteAndUnmarshal("agent.record_event", reqParams, &resp)}
		}()
		select {
		case r := <-ch:
			if r.err != nil {
				return r.err
			}
			// Stop-hook control output: blocks the turn-end and feeds `reason` back
			// to the model so it keeps going until it calls a completion tool.
			if resp.StopDecision != nil && resp.StopDecision.Block {
				out, _ := json.Marshal(map[string]interface{}{
					"decision": "block",
					"reason":   resp.StopDecision.Reason,
				})
				fmt.Fprintln(cmd.OutOrStdout(), string(out))
			}
			return nil
		case <-time.After(2 * time.Second):
			return fmt.Errorf("record-event: server did not respond within 2s")
		}
	},
}
