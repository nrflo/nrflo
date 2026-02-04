package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"nrworkflow/internal/client"
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
	Use:   "add <ticket> <agent-type> <key:value>... | <key> <value>",
	Short: "Add finding(s) for an agent",
	Long: `Add one or more findings for an agent during a workflow phase.

Findings are stored in the workflow state and can be retrieved later.
Values can be JSON (arrays, objects) or plain strings.

Two syntax modes:
  1. Single finding (legacy): <key> <value> as separate arguments
  2. Multiple findings: key:'value' pairs (use quotes for values with spaces)

Examples:
  # Legacy single finding
  nrworkflow findings add TICKET-1 setup-analyzer summary "Initial analysis complete" -w feature

  # Single finding with key:value syntax
  nrworkflow findings add TICKET-1 setup-analyzer summary:'Initial analysis' -w feature

  # Multiple findings at once
  nrworkflow findings add TICKET-1 setup-analyzer summary:'Done' status:'passed' -w feature

  # JSON values
  nrworkflow findings add TICKET-1 setup-analyzer files:'["a.go","b.go"]' -w feature`,
	Args: cobra.MinimumNArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := RequireProject(); err != nil {
			return err
		}
		if err := CheckServer(); err != nil {
			return err
		}

		ticketID := args[0]
		agentType := args[1]

		if findingsAddWorkflow == "" {
			return fmt.Errorf("-w/--workflow is required")
		}

		c := GetClient()

		// Detect syntax mode: if exactly 4 args and 3rd arg doesn't contain ':', use legacy mode
		if len(args) == 4 && !strings.Contains(args[2], ":") {
			// Legacy mode: <key> <value>
			key := args[2]
			value := args[3]

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
		}

		// New mode: key:value pairs
		keyValues := make(map[string]string)
		for i := 2; i < len(args); i++ {
			kv, err := parseKeyValue(args[i])
			if err != nil {
				return fmt.Errorf("invalid key:value format '%s': %w", args[i], err)
			}
			keyValues[kv.key] = kv.value
		}

		params := map[string]interface{}{
			"ticket_id":  ticketID,
			"workflow":   findingsAddWorkflow,
			"agent_type": agentType,
			"key_values": keyValues,
		}
		if findingsAddModel != "" {
			params["model"] = findingsAddModel
		}

		var result struct {
			Status string `json:"status"`
			Count  int    `json:"count"`
		}
		if err := c.ExecuteAndUnmarshal("findings.add-bulk", params, &result); err != nil {
			return fmt.Errorf("failed to add findings: %w", err)
		}

		agentKey := agentType
		if findingsAddModel != "" {
			agentKey = agentType + ":" + findingsAddModel
		}
		fmt.Printf("Added %d finding(s) to %s\n", result.Count, agentKey)
		return nil
	},
}

type keyValuePair struct {
	key   string
	value string
}

// parseKeyValue parses "key:'value'" or "key:value" format
func parseKeyValue(s string) (keyValuePair, error) {
	idx := strings.Index(s, ":")
	if idx == -1 {
		return keyValuePair{}, fmt.Errorf("missing ':' separator")
	}
	if idx == 0 {
		return keyValuePair{}, fmt.Errorf("empty key")
	}

	key := s[:idx]
	value := s[idx+1:]

	// Remove surrounding quotes if present
	if len(value) >= 2 {
		if (value[0] == '\'' && value[len(value)-1] == '\'') ||
			(value[0] == '"' && value[len(value)-1] == '"') {
			value = value[1 : len(value)-1]
		}
	}

	return keyValuePair{key: key, value: value}, nil
}

// Findings get flags
var (
	findingsGetWorkflow string
	findingsGetModel    string
	findingsGetKeys     []string
)

var findingsGetCmd = &cobra.Command{
	Use:   "get <ticket> <agent-type> [key]",
	Short: "Get findings for an agent",
	Long: `Get findings stored by an agent during a workflow phase.

If no key is specified, returns all findings for the agent.
Use -k/--key to fetch specific keys (can be repeated).

For parallel phases with multiple agents:
  - Without --model: returns ALL agents' findings grouped by model ID
  - With --model: returns only that specific agent's findings

Examples:
  # Get all findings for a single agent
  nrworkflow findings get TICKET-1 setup-analyzer -w feature

  # Get specific key using -k flag
  nrworkflow findings get TICKET-1 setup-analyzer -w feature -k summary

  # Get multiple specific keys
  nrworkflow findings get TICKET-1 setup-analyzer -w feature -k summary -k status

  # Legacy: Get specific key as positional argument
  nrworkflow findings get TICKET-1 setup-analyzer summary -w feature

  # Get all parallel agents' findings (grouped by model)
  nrworkflow findings get TICKET-1 setup-analyzer -w parallel-test
  # Returns: {"claude:sonnet": {...}, "codex:gpt_high": {...}}

  # Get specific parallel agent's findings
  nrworkflow findings get TICKET-1 setup-analyzer -w parallel-test --model=claude:sonnet

  # Get specific key from all parallel agents
  nrworkflow findings get TICKET-1 setup-analyzer -w parallel-test -k summary
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

		// Collect keys: from positional arg and/or -k flags
		var keys []string
		if len(args) > 2 {
			keys = append(keys, args[2])
		}
		keys = append(keys, findingsGetKeys...)

		if findingsGetWorkflow == "" {
			return fmt.Errorf("-w/--workflow is required")
		}

		c := GetClient()
		reqParams := map[string]interface{}{
			"ticket_id":  ticketID,
			"workflow":   findingsGetWorkflow,
			"agent_type": agentType,
		}

		// Use single key for backward compat, or keys array for multiple
		if len(keys) == 1 {
			reqParams["key"] = keys[0]
		} else if len(keys) > 1 {
			reqParams["keys"] = keys
		}

		if findingsGetModel != "" {
			reqParams["model"] = findingsGetModel
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
	findingsGetCmd.Flags().StringArrayVarP(&findingsGetKeys, "key", "k", nil, "Key(s) to fetch (can be repeated)")
	findingsCmd.AddCommand(findingsGetCmd)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
