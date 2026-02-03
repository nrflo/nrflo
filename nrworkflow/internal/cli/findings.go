package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"nrworkflow/internal/client"
	"nrworkflow/internal/types"
)

// Add findings command to root
func init() {
	rootCmd.AddCommand(findingsCmd)
}

var findingsCmd = &cobra.Command{
	Use:   "findings",
	Short: "Manage workflow findings",
}

// Findings add flags
var (
	findingsAddWorkflow string
	findingsAddModel    string
)

var findingsAddCmd = &cobra.Command{
	Use:   "add <ticket> <agent-type> <key> <value>",
	Short: "Add a finding for an agent",
	Long: `Add a finding for an agent during a workflow phase.

Findings are stored in the workflow state and can be retrieved later.
Values can be JSON (arrays, objects) or plain strings.

Examples:
  nrworkflow findings add TICKET-1 setup-analyzer summary "Initial analysis complete"
  nrworkflow findings add TICKET-1 setup-analyzer files_to_modify '["src/main.go", "src/util.go"]'`,
	Args: cobra.ExactArgs(4),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := RequireProject(); err != nil {
			return err
		}
		if err := CheckServer(); err != nil {
			return err
		}

		ticketID := args[0]
		agentType := args[1]
		key := args[2]
		value := args[3]

		if findingsAddWorkflow == "" {
			return fmt.Errorf("-w/--workflow is required")
		}

		c := GetClient()
		params := map[string]interface{}{
			"ticket_id":  ticketID,
			"workflow":   findingsAddWorkflow,
			"agent_type": agentType,
			"key":        key,
			"value":      value,
		}
		if findingsAddModel != "" {
			params["model"] = findingsAddModel
		}

		if err := c.ExecuteAndUnmarshal("findings.add", params, nil); err != nil {
			return fmt.Errorf("failed to add finding: %w", err)
		}

		agentKey := agentType
		if findingsAddModel != "" {
			agentKey = agentType + ":" + findingsAddModel
		}
		fmt.Printf("Added finding: %s.%s = %s\n", agentKey, key, truncate(value, 50))
		return nil
	},
}

// Findings get flags
var (
	findingsGetWorkflow string
	findingsGetModel    string
)

var findingsGetCmd = &cobra.Command{
	Use:   "get <ticket> <agent-type> [key]",
	Short: "Get findings for an agent",
	Long: `Get findings stored by an agent during a workflow phase.

If key is omitted, returns all findings for the agent.
If key is provided, returns only that specific finding.

For parallel phases with multiple agents:
  - Without --model: returns ALL agents' findings grouped by model ID
  - With --model: returns only that specific agent's findings

Examples:
  # Get all findings for a single agent
  nrworkflow findings get TICKET-1 setup-analyzer -w feature

  # Get all parallel agents' findings (grouped by model)
  nrworkflow findings get TICKET-1 setup-analyzer -w parallel-test
  # Returns: {"claude:sonnet": {...}, "codex:gpt_high": {...}}

  # Get specific parallel agent's findings
  nrworkflow findings get TICKET-1 setup-analyzer -w parallel-test --model=claude:sonnet

  # Get specific key from all parallel agents
  nrworkflow findings get TICKET-1 setup-analyzer summary -w parallel-test
  # Returns: {"claude:sonnet": "...", "codex:gpt_high": "..."}`,
	Args: cobra.RangeArgs(2, 3),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := RequireProject(); err != nil {
			return err
		}
		if err := CheckServer(); err != nil {
			return err
		}

		ticketID := args[0]
		agentType := args[1]
		var key string
		if len(args) > 2 {
			key = args[2]
		}

		if findingsGetWorkflow == "" {
			return fmt.Errorf("-w/--workflow is required")
		}

		c := GetClient()
		params := types.FindingsGetRequest{
			Workflow:  findingsGetWorkflow,
			AgentType: agentType,
			Key:       key,
			Model:     findingsGetModel,
		}
		reqParams := map[string]interface{}{
			"ticket_id":  ticketID,
			"workflow":   params.Workflow,
			"agent_type": params.AgentType,
			"key":        params.Key,
		}
		if params.Model != "" {
			reqParams["model"] = params.Model
		}

		var result interface{}
		if err := c.ExecuteAndUnmarshal("findings.get", reqParams, &result); err != nil {
			return err
		}

		fmt.Println(client.FormatValue(result))
		return nil
	},
}

func init() {
	findingsAddCmd.Flags().StringVarP(&findingsAddWorkflow, "workflow", "w", "", "Workflow name (required)")
	findingsAddCmd.Flags().StringVar(&findingsAddModel, "model", "", "Model ID for parallel agents")
	findingsCmd.AddCommand(findingsAddCmd)

	findingsGetCmd.Flags().StringVarP(&findingsGetWorkflow, "workflow", "w", "", "Workflow name (required)")
	findingsGetCmd.Flags().StringVar(&findingsGetModel, "model", "", "Model ID for parallel agents")
	findingsCmd.AddCommand(findingsGetCmd)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
