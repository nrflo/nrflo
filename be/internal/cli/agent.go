package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"
)

var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Agent lifecycle commands (used by spawned agents)",
}

// Agent fail/continue/callback flags
var (
	agentFailReason         string
	agentCallbackLevel      int
	agentCallbackTargetAgent string
	agentCallbackChain      []string
	// context-update flags
	agentContextUpdatePctUsed float64
)

var agentFailCmd = &cobra.Command{
	Use:   "fail",
	Short: "Mark the current agent session as failed",
	Long: `Mark the current agent session as failed.

Context is read from environment variables set by the spawner:
  NRF_SESSION_ID          — current agent session ID (required)
  NRF_WORKFLOW_INSTANCE_ID — workflow instance ID

Use --reason to provide a failure description.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := RequireProject(); err != nil {
			return err
		}
		if err := CheckServer(); err != nil {
			return err
		}

		sessionID := GetSessionID()
		if sessionID == "" {
			return fmt.Errorf("NRF_SESSION_ID env var is required")
		}

		c := GetClient()
		reqParams := map[string]interface{}{
			"session_id": sessionID,
		}
		if agentFailReason != "" {
			reqParams["reason"] = agentFailReason
		}
		addSpawnerIDs(reqParams)

		if err := c.ExecuteAndUnmarshal("agent.fail", reqParams, nil); err != nil {
			return err
		}

		fmt.Println("Agent marked as fail")
		return nil
	},
}

var agentFinishedCmd = &cobra.Command{
	Use:   "finished",
	Short: "Mark the current agent session as finished (pass; proceed to next phase)",
	Long: `Mark the current agent session as completed successfully. The orchestrator
will treat this as a PASS and advance to the next phase.

Context is read from environment variables set by the spawner:
  NRF_SESSION_ID          — current agent session ID (required)
  NRF_WORKFLOW_INSTANCE_ID — workflow instance ID`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := RequireProject(); err != nil {
			return err
		}
		if err := CheckServer(); err != nil {
			return err
		}

		sessionID := GetSessionID()
		if sessionID == "" {
			return fmt.Errorf("NRF_SESSION_ID env var is required")
		}

		c := GetClient()
		reqParams := map[string]interface{}{
			"session_id": sessionID,
		}
		addSpawnerIDs(reqParams)

		if err := c.ExecuteAndUnmarshal("agent.finished", reqParams, nil); err != nil {
			return err
		}

		fmt.Println("Agent marked as finished")
		return nil
	},
}

var agentContinueCmd = &cobra.Command{
	Use:   "continue",
	Short: "Signal that the current agent needs context continuation",
	Long: `Signal that the current agent has exhausted its context window and needs to be
relaunched with fresh context to continue the task. Save progress to the
'to_resume' finding before calling this.

Context is read from environment variables set by the spawner:
  NRF_SESSION_ID          — current agent session ID (required)
  NRF_WORKFLOW_INSTANCE_ID — workflow instance ID`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := RequireProject(); err != nil {
			return err
		}
		if err := CheckServer(); err != nil {
			return err
		}

		sessionID := GetSessionID()
		if sessionID == "" {
			return fmt.Errorf("NRF_SESSION_ID env var is required")
		}

		c := GetClient()
		reqParams := map[string]interface{}{
			"session_id": sessionID,
		}
		addSpawnerIDs(reqParams)

		if err := c.ExecuteAndUnmarshal("agent.continue", reqParams, nil); err != nil {
			return err
		}

		fmt.Println("Agent marked as continue (context continuation requested)")
		return nil
	},
}

var agentCallbackCmd = &cobra.Command{
	Use:   "callback",
	Short: "Signal a callback to a previous execution layer, agent, or chain",
	Long: `Signal that the agent needs to callback. Exactly one of --level, --agent, or --chain must be set.

  --level N       callback to layer N (0-based)
  --agent ID      callback targeting a specific agent by ID
  --chain a,b,c   callback targeting a list of phases (comma-separated or repeated flag)

Save callback_instructions as a finding before calling this.

Context is read from environment variables set by the spawner:
  NRF_SESSION_ID          — current agent session ID (required)
  NRF_WORKFLOW_INSTANCE_ID — workflow instance ID`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		levelSet := cmd.Flags().Changed("level")
		agentSet := agentCallbackTargetAgent != ""
		chainSet := len(agentCallbackChain) > 0

		setCount := 0
		if levelSet {
			setCount++
		}
		if agentSet {
			setCount++
		}
		if chainSet {
			setCount++
		}
		if setCount != 1 {
			return fmt.Errorf("exactly one of --level, --agent, or --chain must be set")
		}

		if err := RequireProject(); err != nil {
			return err
		}
		if err := CheckServer(); err != nil {
			return err
		}

		sessionID := GetSessionID()
		if sessionID == "" {
			return fmt.Errorf("NRF_SESSION_ID env var is required")
		}

		c := GetClient()
		reqParams := map[string]interface{}{
			"session_id": sessionID,
		}
		addSpawnerIDs(reqParams)

		switch {
		case agentSet:
			reqParams["mode"] = "agent"
			reqParams["target_agent"] = agentCallbackTargetAgent
		case chainSet:
			reqParams["mode"] = "chain"
			reqParams["chain"] = agentCallbackChain
		default:
			if agentCallbackLevel < 0 {
				return fmt.Errorf("--level must be >= 0")
			}
			reqParams["mode"] = "layer"
			reqParams["level"] = agentCallbackLevel
		}

		if err := c.ExecuteAndUnmarshal("agent.callback", reqParams, nil); err != nil {
			return err
		}

		switch {
		case agentSet:
			fmt.Printf("Agent marked as callback (target agent: %s)\n", agentCallbackTargetAgent)
		case chainSet:
			fmt.Printf("Agent marked as callback (target chain: %v)\n", agentCallbackChain)
		default:
			fmt.Printf("Agent marked as callback (target layer: %d)\n", agentCallbackLevel)
		}
		return nil
	},
}

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
		type result struct{ err error }
		ch := make(chan result, 1)
		go func() {
			ch <- result{err: GetClient().ExecuteAndUnmarshal("agent.record_event", reqParams, nil)}
		}()
		select {
		case r := <-ch:
			return r.err
		case <-time.After(2 * time.Second):
			return fmt.Errorf("record-event: server did not respond within 2s")
		}
	},
}

func init() {
	// agent fail
	agentFailCmd.Flags().StringVar(&agentFailReason, "reason", "", "Failure reason")
	agentCmd.AddCommand(agentFailCmd)

	// agent finished
	agentCmd.AddCommand(agentFinishedCmd)

	// agent continue
	agentCmd.AddCommand(agentContinueCmd)

	// agent callback
	agentCallbackCmd.Flags().IntVar(&agentCallbackLevel, "level", 0, "Target layer index to callback to")
	agentCallbackCmd.Flags().StringVar(&agentCallbackTargetAgent, "agent", "", "Target agent ID to callback to")
	agentCallbackCmd.Flags().StringSliceVar(&agentCallbackChain, "chain", nil, "Target phases for chain callback (comma-separated)")
	agentCmd.AddCommand(agentCallbackCmd)

	// agent context-update
	agentContextUpdateCmd.Flags().Float64Var(&agentContextUpdatePctUsed, "pct-used", 0, "Percentage of context used")
	agentCmd.AddCommand(agentContextUpdateCmd)

	// agent record-event
	agentCmd.AddCommand(agentRecordEventCmd)
}
