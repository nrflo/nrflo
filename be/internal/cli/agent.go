package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"be/internal/types"
)

// Add agent command to root
func init() {
	rootCmd.AddCommand(agentCmd)
}

var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Agent lifecycle commands (used by spawned agents)",
}

// Agent complete/fail/continue flags
var (
	agentCompleteWorkflow string
	agentCompleteModel    string
	agentFailWorkflow     string
	agentFailModel        string
	agentFailReason       string
	agentContinueWorkflow string
	agentContinueModel    string
)

var agentCompleteCmd = &cobra.Command{
	Use:   "complete <ticket> <agent-type>",
	Short: "Mark an agent as completed successfully",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := RequireProject(); err != nil {
			return err
		}
		if err := CheckServer(); err != nil {
			return err
		}

		ticketID := args[0]
		agentType := args[1]

		if agentCompleteWorkflow == "" {
			return fmt.Errorf("-w/--workflow is required")
		}

		c := GetClient()
		params := types.AgentCompleteRequest{
			Workflow:  agentCompleteWorkflow,
			AgentType: agentType,
			Model:     agentCompleteModel,
		}
		reqParams := map[string]interface{}{
			"ticket_id":  ticketID,
			"workflow":   params.Workflow,
			"agent_type": params.AgentType,
		}
		if params.Model != "" {
			reqParams["model"] = params.Model
		}

		if err := c.ExecuteAndUnmarshal("agent.complete", reqParams, nil); err != nil {
			return err
		}

		fmt.Printf("Agent %s marked as pass\n", agentType)
		return nil
	},
}

var agentFailCmd = &cobra.Command{
	Use:   "fail <ticket> <agent-type>",
	Short: "Mark an agent as failed",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := RequireProject(); err != nil {
			return err
		}
		if err := CheckServer(); err != nil {
			return err
		}

		ticketID := args[0]
		agentType := args[1]

		if agentFailWorkflow == "" {
			return fmt.Errorf("-w/--workflow is required")
		}

		c := GetClient()
		params := types.AgentCompleteRequest{
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

		if err := c.ExecuteAndUnmarshal("agent.fail", reqParams, nil); err != nil {
			return err
		}

		fmt.Printf("Agent %s marked as fail\n", agentType)
		return nil
	},
}

var agentContinueCmd = &cobra.Command{
	Use:   "continue <ticket> <agent-type>",
	Short: "Signal that an agent needs context continuation",
	Long: `Signal that an agent has exhausted its context window and needs to be
relaunched with fresh context to continue the task. The spawner will
automatically relaunch the agent if max_continuations has not been reached.`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := RequireProject(); err != nil {
			return err
		}
		if err := CheckServer(); err != nil {
			return err
		}

		ticketID := args[0]
		agentType := args[1]

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

		if err := c.ExecuteAndUnmarshal("agent.continue", reqParams, nil); err != nil {
			return err
		}

		fmt.Printf("Agent %s marked as continue (context continuation requested)\n", agentType)
		return nil
	},
}

func init() {
	// agent complete
	agentCompleteCmd.Flags().StringVarP(&agentCompleteWorkflow, "workflow", "w", "", "Workflow name (required)")
	agentCompleteCmd.Flags().StringVar(&agentCompleteModel, "model", "", "Model ID")
	agentCmd.AddCommand(agentCompleteCmd)

	// agent fail
	agentFailCmd.Flags().StringVarP(&agentFailWorkflow, "workflow", "w", "", "Workflow name (required)")
	agentFailCmd.Flags().StringVar(&agentFailModel, "model", "", "Model ID")
	agentFailCmd.Flags().StringVar(&agentFailReason, "reason", "", "Failure reason")
	agentCmd.AddCommand(agentFailCmd)

	// agent continue
	agentContinueCmd.Flags().StringVarP(&agentContinueWorkflow, "workflow", "w", "", "Workflow name (required)")
	agentContinueCmd.Flags().StringVar(&agentContinueModel, "model", "", "Model ID")
	agentCmd.AddCommand(agentContinueCmd)
}
