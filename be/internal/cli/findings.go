package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"be/internal/client"
)

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

// Findings append flags
var (
	findingsAppendWorkflow string
	findingsAppendModel    string
)

var findingsAppendCmd = &cobra.Command{
	Use:   "append <ticket> <agent-type> <key:value>... | <key> <value>",
	Short: "Append value(s) to finding(s)",
	Long: `Append one or more values to existing findings (creating arrays if needed).

Append logic:
  - If key doesn't exist: store value as-is
  - If existing is array AND new is array: flatten (merge arrays)
  - If existing is array AND new is not array: append element
  - If existing is not array: convert to [existing, new]

Two syntax modes:
  1. Single append (legacy): <key> <value> as separate arguments
  2. Multiple appends: key:'value' pairs

Examples:
  # Append to a key (creates array if needed)
  nrworkflow findings append TICKET-1 setup-analyzer files:'src/main.go' -w feature
  nrworkflow findings append TICKET-1 setup-analyzer files:'src/util.go' -w feature
  # Result: files = ["src/main.go", "src/util.go"]

  # Append array (flattens)
  nrworkflow findings append TICKET-1 setup-analyzer files:'["a.go","b.go"]' -w feature
  # If files was ["main.go"], now: ["main.go", "a.go", "b.go"]

  # Multiple appends at once
  nrworkflow findings append TICKET-1 setup-analyzer files:'new.go' errors:'error1' -w feature`,
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

		if findingsAppendWorkflow == "" {
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
				"workflow":   findingsAppendWorkflow,
				"agent_type": agentType,
				"key":        key,
				"value":      value,
			}
			if findingsAppendModel != "" {
				params["model"] = findingsAppendModel
			}

			if err := c.ExecuteAndUnmarshal("findings.append", params, nil); err != nil {
				return fmt.Errorf("failed to append finding: %w", err)
			}

			agentKey := agentType
			if findingsAppendModel != "" {
				agentKey = agentType + ":" + findingsAppendModel
			}
			fmt.Printf("Appended to finding: %s.%s\n", agentKey, key)
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
			"workflow":   findingsAppendWorkflow,
			"agent_type": agentType,
			"key_values": keyValues,
		}
		if findingsAppendModel != "" {
			params["model"] = findingsAppendModel
		}

		var result struct {
			Status string `json:"status"`
			Count  int    `json:"count"`
		}
		if err := c.ExecuteAndUnmarshal("findings.append-bulk", params, &result); err != nil {
			return fmt.Errorf("failed to append findings: %w", err)
		}

		agentKey := agentType
		if findingsAppendModel != "" {
			agentKey = agentType + ":" + findingsAppendModel
		}
		fmt.Printf("Appended to %d finding(s) in %s\n", result.Count, agentKey)
		return nil
	},
}

// Findings delete flags
var (
	findingsDeleteWorkflow string
	findingsDeleteModel    string
)

var findingsDeleteCmd = &cobra.Command{
	Use:   "delete <ticket> <agent-type> <key>...",
	Short: "Delete finding key(s)",
	Long: `Delete one or more finding keys from an agent.

Examples:
  # Delete a single key
  nrworkflow findings delete TICKET-1 setup-analyzer summary -w feature

  # Delete multiple keys
  nrworkflow findings delete TICKET-1 setup-analyzer summary status files -w feature

  # Delete from specific parallel agent
  nrworkflow findings delete TICKET-1 setup-analyzer summary -w feature --model=claude:sonnet`,
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
		keys := args[2:]

		if findingsDeleteWorkflow == "" {
			return fmt.Errorf("-w/--workflow is required")
		}

		c := GetClient()

		params := map[string]interface{}{
			"ticket_id":  ticketID,
			"workflow":   findingsDeleteWorkflow,
			"agent_type": agentType,
			"keys":       keys,
		}
		if findingsDeleteModel != "" {
			params["model"] = findingsDeleteModel
		}

		var result struct {
			Status  string `json:"status"`
			Deleted int    `json:"deleted"`
		}
		if err := c.ExecuteAndUnmarshal("findings.delete", params, &result); err != nil {
			return fmt.Errorf("failed to delete findings: %w", err)
		}

		agentKey := agentType
		if findingsDeleteModel != "" {
			agentKey = agentType + ":" + findingsDeleteModel
		}
		fmt.Printf("Deleted %d finding(s) from %s\n", result.Deleted, agentKey)
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

	findingsAppendCmd.Flags().StringVarP(&findingsAppendWorkflow, "workflow", "w", "", "Workflow name (required)")
	findingsAppendCmd.Flags().StringVar(&findingsAppendModel, "model", "", "Model ID for parallel agents")
	findingsCmd.AddCommand(findingsAppendCmd)

	findingsDeleteCmd.Flags().StringVarP(&findingsDeleteWorkflow, "workflow", "w", "", "Workflow name (required)")
	findingsDeleteCmd.Flags().StringVar(&findingsDeleteModel, "model", "", "Model ID for parallel agents")
	findingsCmd.AddCommand(findingsDeleteCmd)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
