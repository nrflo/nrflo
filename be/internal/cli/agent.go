package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"be/internal/types"
)

var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Agent lifecycle commands (used by spawned agents)",
}

// Shared no-ticket flag for project-scoped workflows
var agentNoTicket bool

// Agent fail/continue/callback flags
var (
	agentFailWorkflow string
	agentFailModel        string
	agentFailReason       string
	agentContinueWorkflow string
	agentContinueModel    string
	agentCallbackWorkflow string
	agentCallbackModel    string
	agentCallbackLevel    int
	// context-update flags
	agentContextUpdatePctUsed float64
)

var agentFailCmd = &cobra.Command{
	Use:   "fail [-T] [<ticket>] <agent-type>",
	Short: "Mark an agent as failed",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := RequireProject(); err != nil {
			return err
		}
		if err := CheckServer(); err != nil {
			return err
		}

		ticketID, agentType := parseAgentArgs(args, agentNoTicket)

		if agentFailWorkflow == "" {
			return fmt.Errorf("-w/--workflow is required")
		}

		c := GetClient()
		params := types.AgentRequest{
			Workflow:  agentFailWorkflow,
			AgentType: agentType,
			Model:     agentFailModel,
		}
		reqParams := map[string]interface{}{
			"ticket_id":  ticketID,
			"workflow":   params.Workflow,
			"agent_type": params.AgentType,
		}
		if params.Model != "" {
			reqParams["model"] = params.Model
		}
		addSpawnerIDs(reqParams)

		if err := c.ExecuteAndUnmarshal("agent.fail", reqParams, nil); err != nil {
			return err
		}

		fmt.Printf("Agent %s marked as fail\n", agentType)
		return nil
	},
}

var agentContinueCmd = &cobra.Command{
	Use:   "continue [-T] [<ticket>] <agent-type>",
	Short: "Signal that an agent needs context continuation",
	Long: `Signal that an agent has exhausted its context window and needs to be
relaunched with fresh context to continue the task. The spawner will
automatically relaunch the agent if max_continuations has not been reached.
Use -T/--no-ticket for project-scoped workflows.`,
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := RequireProject(); err != nil {
			return err
		}
		if err := CheckServer(); err != nil {
			return err
		}

		ticketID, agentType := parseAgentArgs(args, agentNoTicket)

		if agentContinueWorkflow == "" {
			return fmt.Errorf("-w/--workflow is required")
		}

		c := GetClient()
		reqParams := map[string]interface{}{
			"ticket_id":  ticketID,
			"workflow":   agentContinueWorkflow,
			"agent_type": agentType,
		}
		if agentContinueModel != "" {
			reqParams["model"] = agentContinueModel
		}
		addSpawnerIDs(reqParams)

		if err := c.ExecuteAndUnmarshal("agent.continue", reqParams, nil); err != nil {
			return err
		}

		fmt.Printf("Agent %s marked as continue (context continuation requested)\n", agentType)
		return nil
	},
}

var agentCallbackCmd = &cobra.Command{
	Use:   "callback [-T] [<ticket>] <agent-type>",
	Short: "Signal a callback to a previous execution layer",
	Long: `Signal that the agent needs to callback to a previous layer for fixes.
The --level flag specifies the target layer index (0-based) to callback to.
The agent should save callback_instructions as a finding before calling this.
Use -T/--no-ticket for project-scoped workflows.`,
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := RequireProject(); err != nil {
			return err
		}
		if err := CheckServer(); err != nil {
			return err
		}

		ticketID, agentType := parseAgentArgs(args, agentNoTicket)

		if agentCallbackWorkflow == "" {
			return fmt.Errorf("-w/--workflow is required")
		}

		if !cmd.Flags().Changed("level") {
			return fmt.Errorf("--level is required")
		}
		if agentCallbackLevel < 0 {
			return fmt.Errorf("--level must be >= 0")
		}

		c := GetClient()
		reqParams := map[string]interface{}{
			"ticket_id":  ticketID,
			"workflow":   agentCallbackWorkflow,
			"agent_type": agentType,
			"level":      agentCallbackLevel,
		}
		if agentCallbackModel != "" {
			reqParams["model"] = agentCallbackModel
		}
		addSpawnerIDs(reqParams)

		if err := c.ExecuteAndUnmarshal("agent.callback", reqParams, nil); err != nil {
			return err
		}

		fmt.Printf("Agent %s marked as callback (target layer: %d)\n", agentType, agentCallbackLevel)
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

// parseAgentArgs extracts ticketID and agentType from positional args.
// When noTicket is true, ticketID is "" and args[0] is agentType.
func parseAgentArgs(args []string, noTicket bool) (ticketID, agentType string) {
	if noTicket {
		return "", args[0]
	}
	return args[0], args[1]
}

func init() {
	for _, cmd := range []*cobra.Command{agentFailCmd, agentContinueCmd, agentCallbackCmd} {
		cmd.Flags().BoolVarP(&agentNoTicket, "no-ticket", "T", false, "Project-scoped workflow (no ticket ID)")
	}

	// agent fail
	agentFailCmd.Flags().StringVarP(&agentFailWorkflow, "workflow", "w", "", "Workflow name (required)")
	agentFailCmd.Flags().StringVar(&agentFailModel, "model", "", "Model ID")
	agentFailCmd.Flags().StringVar(&agentFailReason, "reason", "", "Failure reason")
	agentCmd.AddCommand(agentFailCmd)

	// agent continue
	agentContinueCmd.Flags().StringVarP(&agentContinueWorkflow, "workflow", "w", "", "Workflow name (required)")
	agentContinueCmd.Flags().StringVar(&agentContinueModel, "model", "", "Model ID")
	agentCmd.AddCommand(agentContinueCmd)

	// agent callback
	agentCallbackCmd.Flags().StringVarP(&agentCallbackWorkflow, "workflow", "w", "", "Workflow name (required)")
	agentCallbackCmd.Flags().StringVar(&agentCallbackModel, "model", "", "Model ID")
	agentCallbackCmd.Flags().IntVar(&agentCallbackLevel, "level", 0, "Target layer index to callback to (required)")
	agentCmd.AddCommand(agentCallbackCmd)

	// agent context-update
	agentContextUpdateCmd.Flags().Float64Var(&agentContextUpdatePctUsed, "pct-used", 0, "Percentage of context used")
	agentCmd.AddCommand(agentContextUpdateCmd)
}
