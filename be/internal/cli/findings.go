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

// Shared no-ticket flag for project-scoped workflows
var findingsNoTicket bool

// Findings add flags
var (
	findingsAddWorkflow string
	findingsAddModel    string
)

var findingsAddCmd = &cobra.Command{
	Use:   "add [-T] [<ticket>] <agent-type> <key:value>... | <key> <value>",
	Short: "Add finding(s) for an agent",
	Long: `Add one or more findings for an agent during a workflow phase.

Findings are stored in the workflow state and can be retrieved later.
Values can be JSON (arrays, objects) or plain strings.

Use -T/--no-ticket for project-scoped workflows (no ticket ID needed).

Two syntax modes:
  1. Single finding (legacy): <key> <value> as separate arguments
  2. Multiple findings: key:'value' pairs (use quotes for values with spaces)

Examples:
  # Legacy single finding
  nrworkflow findings add TICKET-1 setup-analyzer summary "Initial analysis complete" -w feature

  # Multiple findings at once
  nrworkflow findings add TICKET-1 setup-analyzer summary:'Done' status:'passed' -w feature

  # Project-scoped (no ticket)
  nrworkflow findings add -T consolidate summary:'Done' -w learning`,
	Args: cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := RequireProject(); err != nil {
			return err
		}
		if err := CheckServer(); err != nil {
			return err
		}

		ticketID, agentType, rest := parseFindingsArgs(args, findingsNoTicket)
		if len(rest) < 1 {
			return fmt.Errorf("requires at least one key:value pair after agent-type")
		}

		if findingsAddWorkflow == "" {
			return fmt.Errorf("-w/--workflow is required")
		}

		c := GetClient()

		// Detect syntax mode: if exactly 2 rest args and 1st doesn't contain ':', use legacy mode
		if len(rest) == 2 && !strings.Contains(rest[0], ":") {
			key := rest[0]
			value := rest[1]

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
		for _, arg := range rest {
			kv, err := parseKeyValue(arg)
			if err != nil {
				return fmt.Errorf("invalid key:value format '%s': %w", arg, err)
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
	Use:   "get [-T] [<ticket>] <agent-type> [key]",
	Short: "Get findings for an agent",
	Long: `Get findings stored by an agent during a workflow phase.

If no key is specified, returns all findings for the agent.
Use -k/--key to fetch specific keys (can be repeated).
Use -T/--no-ticket for project-scoped workflows (no ticket ID needed).

Examples:
  # Get all findings for a single agent
  nrworkflow findings get TICKET-1 setup-analyzer -w feature

  # Get specific key
  nrworkflow findings get TICKET-1 setup-analyzer -w feature -k summary

  # Project-scoped (no ticket)
  nrworkflow findings get -T consolidate -w learning -k summary`,
	Args: cobra.RangeArgs(1, 3),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := RequireProject(); err != nil {
			return err
		}
		if err := CheckServer(); err != nil {
			return err
		}

		ticketID, agentType, rest := parseFindingsArgs(args, findingsNoTicket)

		// Collect keys: from positional arg and/or -k flags
		var keys []string
		if len(rest) > 0 {
			keys = append(keys, rest[0])
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
	Use:   "append [-T] [<ticket>] <agent-type> <key:value>... | <key> <value>",
	Short: "Append value(s) to finding(s)",
	Long: `Append one or more values to existing findings (creating arrays if needed).

Use -T/--no-ticket for project-scoped workflows (no ticket ID needed).

Two syntax modes:
  1. Single append (legacy): <key> <value> as separate arguments
  2. Multiple appends: key:'value' pairs

Examples:
  # Append to a key (creates array if needed)
  nrworkflow findings append TICKET-1 setup-analyzer files:'src/main.go' -w feature

  # Project-scoped (no ticket)
  nrworkflow findings append -T consolidate files:'src/main.go' -w learning`,
	Args: cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := RequireProject(); err != nil {
			return err
		}
		if err := CheckServer(); err != nil {
			return err
		}

		ticketID, agentType, rest := parseFindingsArgs(args, findingsNoTicket)
		if len(rest) < 1 {
			return fmt.Errorf("requires at least one key:value pair after agent-type")
		}

		if findingsAppendWorkflow == "" {
			return fmt.Errorf("-w/--workflow is required")
		}

		c := GetClient()

		// Detect syntax mode: if exactly 2 rest args and 1st doesn't contain ':', use legacy mode
		if len(rest) == 2 && !strings.Contains(rest[0], ":") {
			key := rest[0]
			value := rest[1]

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
		for _, arg := range rest {
			kv, err := parseKeyValue(arg)
			if err != nil {
				return fmt.Errorf("invalid key:value format '%s': %w", arg, err)
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
	Use:   "delete [-T] [<ticket>] <agent-type> <key>...",
	Short: "Delete finding key(s)",
	Long: `Delete one or more finding keys from an agent.

Use -T/--no-ticket for project-scoped workflows (no ticket ID needed).

Examples:
  # Delete a single key
  nrworkflow findings delete TICKET-1 setup-analyzer summary -w feature

  # Project-scoped (no ticket)
  nrworkflow findings delete -T consolidate summary -w learning`,
	Args: cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := RequireProject(); err != nil {
			return err
		}
		if err := CheckServer(); err != nil {
			return err
		}

		ticketID, agentType, rest := parseFindingsArgs(args, findingsNoTicket)
		if len(rest) < 1 {
			return fmt.Errorf("requires at least one key to delete after agent-type")
		}

		if findingsDeleteWorkflow == "" {
			return fmt.Errorf("-w/--workflow is required")
		}

		c := GetClient()

		params := map[string]interface{}{
			"ticket_id":  ticketID,
			"workflow":   findingsDeleteWorkflow,
			"agent_type": agentType,
			"keys":       rest,
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
	for _, cmd := range []*cobra.Command{findingsAddCmd, findingsGetCmd, findingsAppendCmd, findingsDeleteCmd} {
		cmd.Flags().BoolVarP(&findingsNoTicket, "no-ticket", "T", false, "Project-scoped workflow (no ticket ID)")
	}

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

// parseFindingsArgs extracts ticketID, agentType, and remaining args.
// When noTicket is true, ticketID is "" and args[0] is agentType.
func parseFindingsArgs(args []string, noTicket bool) (ticketID, agentType string, rest []string) {
	if noTicket {
		return "", args[0], args[1:]
	}
	return args[0], args[1], args[2:]
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
