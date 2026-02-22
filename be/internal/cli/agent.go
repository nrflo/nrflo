package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Agent lifecycle commands (used by spawned agents)",
}

// Agent fail/continue/callback flags
var (
	agentFailReason    string
	agentCallbackLevel int
	// context-update flags
	agentContextUpdatePctUsed float64
)

var agentFailCmd = &cobra.Command{
	Use:   "fail",
	Short: "Mark the current agent session as failed",
	Long: `Mark the current agent session as failed.

Context is read from environment variables set by the spawner:
  NRWF_SESSION_ID          — current agent session ID (required)
  NRWF_WORKFLOW_INSTANCE_ID — workflow instance ID

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
			return fmt.Errorf("NRWF_SESSION_ID env var is required")
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

var agentContinueCmd = &cobra.Command{
	Use:   "continue",
	Short: "Signal that the current agent needs context continuation",
	Long: `Signal that the current agent has exhausted its context window and needs to be
relaunched with fresh context to continue the task. Save progress to the
'to_resume' finding before calling this.

Context is read from environment variables set by the spawner:
  NRWF_SESSION_ID          — current agent session ID (required)
  NRWF_WORKFLOW_INSTANCE_ID — workflow instance ID`,
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
			return fmt.Errorf("NRWF_SESSION_ID env var is required")
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
	Short: "Signal a callback to a previous execution layer",
	Long: `Signal that the agent needs to callback to a previous layer for fixes.
The --level flag specifies the target layer index (0-based) to callback to.
Save callback_instructions as a finding before calling this.

Context is read from environment variables set by the spawner:
  NRWF_SESSION_ID          — current agent session ID (required)
  NRWF_WORKFLOW_INSTANCE_ID — workflow instance ID`,
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
			return fmt.Errorf("NRWF_SESSION_ID env var is required")
		}

		if !cmd.Flags().Changed("level") {
			return fmt.Errorf("--level is required")
		}
		if agentCallbackLevel < 0 {
			return fmt.Errorf("--level must be >= 0")
		}

		c := GetClient()
		reqParams := map[string]interface{}{
			"session_id": sessionID,
			"level":      agentCallbackLevel,
		}
		addSpawnerIDs(reqParams)

		if err := c.ExecuteAndUnmarshal("agent.callback", reqParams, nil); err != nil {
			return err
		}

		fmt.Printf("Agent marked as callback (target layer: %d)\n", agentCallbackLevel)
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

func init() {
	// agent fail
	agentFailCmd.Flags().StringVar(&agentFailReason, "reason", "", "Failure reason")
	agentCmd.AddCommand(agentFailCmd)

	// agent continue
	agentCmd.AddCommand(agentContinueCmd)

	// agent callback
	agentCallbackCmd.Flags().IntVar(&agentCallbackLevel, "level", 0, "Target layer index to callback to (required)")
	agentCmd.AddCommand(agentCallbackCmd)

	// agent context-update
	agentContextUpdateCmd.Flags().Float64Var(&agentContextUpdatePctUsed, "pct-used", 0, "Percentage of context used")
	agentCmd.AddCommand(agentContextUpdateCmd)
}
