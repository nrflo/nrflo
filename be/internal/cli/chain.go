package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	chainNextInstructions string
	chainNextTicketID     string
)

var chainNextInstructionsCmd = &cobra.Command{
	Use:   "chain-next-instructions",
	Short: "Set instructions for the next step in the current workflow chain run",
	Long: `Store instructions that the next step in the chain will receive as its
initial instructions. Must be called before the current agent exits (before
'agent finished' or exit 0).

Context is read from environment variables set by the spawner:
  NRF_WORKFLOW_INSTANCE_ID — workflow instance ID (required)`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := RequireProject(); err != nil {
			return err
		}
		if err := CheckServer(); err != nil {
			return err
		}

		instanceID := GetWorkflowInstanceID()
		if instanceID == "" {
			return fmt.Errorf("NRF_WORKFLOW_INSTANCE_ID env var is required")
		}
		if chainNextInstructions == "" {
			return fmt.Errorf("--instructions is required")
		}

		c := GetClient()
		reqParams := map[string]interface{}{
			"instance_id":  instanceID,
			"instructions": chainNextInstructions,
		}
		addSpawnerIDs(reqParams)

		if err := c.ExecuteAndUnmarshal("agent.chain_next_instructions", reqParams, nil); err != nil {
			return err
		}
		fmt.Println("Next step instructions set")
		return nil
	},
}

var chainNextTicketCmd = &cobra.Command{
	Use:   "chain-next-ticket",
	Short: "Set the ticket ID for the next ticket-scope step in the current workflow chain run",
	Long: `Store the ticket ID that the next ticket-scope step will operate on.
Must be called before the current agent exits (before 'agent finished' or exit 0).

Context is read from environment variables set by the spawner:
  NRF_WORKFLOW_INSTANCE_ID — workflow instance ID (required)`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := RequireProject(); err != nil {
			return err
		}
		if err := CheckServer(); err != nil {
			return err
		}

		instanceID := GetWorkflowInstanceID()
		if instanceID == "" {
			return fmt.Errorf("NRF_WORKFLOW_INSTANCE_ID env var is required")
		}
		if chainNextTicketID == "" {
			return fmt.Errorf("--ticket-id is required")
		}

		c := GetClient()
		reqParams := map[string]interface{}{
			"instance_id": instanceID,
			"ticket_id":   chainNextTicketID,
		}
		addSpawnerIDs(reqParams)

		if err := c.ExecuteAndUnmarshal("agent.chain_next_ticket", reqParams, nil); err != nil {
			return err
		}
		fmt.Printf("Next step ticket ID set to %s\n", chainNextTicketID)
		return nil
	},
}

func init() {
	chainNextInstructionsCmd.Flags().StringVar(&chainNextInstructions, "instructions", "", "Instructions for the next step (required)")
	agentCmd.AddCommand(chainNextInstructionsCmd)

	chainNextTicketCmd.Flags().StringVar(&chainNextTicketID, "ticket-id", "", "Ticket ID for the next ticket-scope step (required)")
	agentCmd.AddCommand(chainNextTicketCmd)
}
