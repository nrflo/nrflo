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

// Findings get flags
var findingsGetKeys []string

var findingsAddCmd = &cobra.Command{
	Use:   "add <key:value>... | <key> <value>",
	Short: "Add finding(s) to the current agent session",
	Long: `Add one or more findings to the current agent session.

Context is read from environment variables set by the spawner:
  NRF_SESSION_ID          — current agent session ID (required)
  NRF_WORKFLOW_INSTANCE_ID — workflow instance ID

Two syntax modes:
  1. Single finding: <key> <value> as separate arguments
  2. Multiple findings: key:'value' pairs (use quotes for values with spaces)

Examples:
  nrflow findings add summary "Initial analysis complete"
  nrflow findings add summary:'Done' status:'passed'`,
	Args: cobra.MinimumNArgs(1),
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

		// Detect syntax mode: if exactly 2 args and 1st doesn't contain ':', use single-value mode
		if len(args) == 2 && !strings.Contains(args[0], ":") {
			key := args[0]
			value := args[1]

			params := map[string]interface{}{
				"session_id": sessionID,
				"key":        key,
				"value":      value,
			}
			addSpawnerIDs(params)

			if err := c.ExecuteAndUnmarshal("findings.add", params, nil); err != nil {
				return fmt.Errorf("failed to add finding: %w", err)
			}

			fmt.Printf("Added finding: %s = %s\n", key, truncate(value, 50))
			return nil
		}

		// Key:value pairs mode
		keyValues := make(map[string]string)
		for _, arg := range args {
			kv, err := parseKeyValue(arg)
			if err != nil {
				return fmt.Errorf("invalid key:value format '%s': %w", arg, err)
			}
			keyValues[kv.key] = kv.value
		}

		params := map[string]interface{}{
			"session_id": sessionID,
			"key_values": keyValues,
		}
		addSpawnerIDs(params)

		var result struct {
			Status string `json:"status"`
			Count  int    `json:"count"`
		}
		if err := c.ExecuteAndUnmarshal("findings.add-bulk", params, &result); err != nil {
			return fmt.Errorf("failed to add findings: %w", err)
		}

		fmt.Printf("Added %d finding(s)\n", result.Count)
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

var findingsGetCmd = &cobra.Command{
	Use:   "get [<agent-type>] [key]",
	Short: "Get findings for an agent or the current session",
	Long: `Get findings stored by an agent.

If no agent-type is given, returns findings from the current session (env NRF_SESSION_ID).
If agent-type is given, reads cross-agent findings (env NRF_WORKFLOW_INSTANCE_ID required).
Use -k/--key to filter specific keys (can be repeated).

Examples:
  # Own session — all findings
  nrflow findings get

  # Own session — specific key
  nrflow findings get -k summary

  # Cross-agent — all findings from setup-analyzer
  nrflow findings get setup-analyzer

  # Cross-agent — specific key
  nrflow findings get setup-analyzer summary`,
	Args: cobra.RangeArgs(0, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := RequireProject(); err != nil {
			return err
		}
		if err := CheckServer(); err != nil {
			return err
		}

		var agentType, positionalKey string
		if len(args) >= 1 {
			agentType = args[0]
		}
		if len(args) >= 2 {
			positionalKey = args[1]
		}

		// Collect keys: from positional arg and/or -k flags
		var keys []string
		if positionalKey != "" {
			keys = append(keys, positionalKey)
		}
		keys = append(keys, findingsGetKeys...)

		c := GetClient()
		reqParams := map[string]interface{}{}
		if agentType != "" {
			reqParams["agent_type"] = agentType
		}

		// Use single key for backward compat, or keys array for multiple
		if len(keys) == 1 {
			reqParams["key"] = keys[0]
		} else if len(keys) > 1 {
			reqParams["keys"] = keys
		}

		addSpawnerIDs(reqParams)

		var result interface{}
		if err := c.ExecuteAndUnmarshal("findings.get", reqParams, &result); err != nil {
			return err
		}

		fmt.Println(client.FormatValue(result))
		return nil
	},
}

var findingsAppendCmd = &cobra.Command{
	Use:   "append <key:value>... | <key> <value>",
	Short: "Append value(s) to finding(s) in the current agent session",
	Long: `Append one or more values to existing findings (creating arrays if needed).

Context is read from environment variables set by the spawner:
  NRF_SESSION_ID          — current agent session ID (required)
  NRF_WORKFLOW_INSTANCE_ID — workflow instance ID

Examples:
  nrflow findings append files:'src/main.go'
  nrflow findings append files:'src/main.go' tests:'src/main_test.go'`,
	Args: cobra.MinimumNArgs(1),
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

		// Detect syntax mode: if exactly 2 args and 1st doesn't contain ':', use single-value mode
		if len(args) == 2 && !strings.Contains(args[0], ":") {
			key := args[0]
			value := args[1]

			params := map[string]interface{}{
				"session_id": sessionID,
				"key":        key,
				"value":      value,
			}
			addSpawnerIDs(params)

			if err := c.ExecuteAndUnmarshal("findings.append", params, nil); err != nil {
				return fmt.Errorf("failed to append finding: %w", err)
			}

			fmt.Printf("Appended to finding: %s\n", key)
			return nil
		}

		// Key:value pairs mode
		keyValues := make(map[string]string)
		for _, arg := range args {
			kv, err := parseKeyValue(arg)
			if err != nil {
				return fmt.Errorf("invalid key:value format '%s': %w", arg, err)
			}
			keyValues[kv.key] = kv.value
		}

		params := map[string]interface{}{
			"session_id": sessionID,
			"key_values": keyValues,
		}
		addSpawnerIDs(params)

		var result struct {
			Status string `json:"status"`
			Count  int    `json:"count"`
		}
		if err := c.ExecuteAndUnmarshal("findings.append-bulk", params, &result); err != nil {
			return fmt.Errorf("failed to append findings: %w", err)
		}

		fmt.Printf("Appended to %d finding(s)\n", result.Count)
		return nil
	},
}

var findingsDeleteCmd = &cobra.Command{
	Use:   "delete <key>...",
	Short: "Delete finding key(s) from the current agent session",
	Long: `Delete one or more finding keys from the current agent session.

Context is read from environment variables set by the spawner:
  NRF_SESSION_ID          — current agent session ID (required)
  NRF_WORKFLOW_INSTANCE_ID — workflow instance ID

Examples:
  nrflow findings delete summary
  nrflow findings delete temp_notes draft_output`,
	Args: cobra.MinimumNArgs(1),
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

		params := map[string]interface{}{
			"session_id": sessionID,
			"keys":       args,
		}
		addSpawnerIDs(params)

		var result struct {
			Status  string `json:"status"`
			Deleted int    `json:"deleted"`
		}
		if err := c.ExecuteAndUnmarshal("findings.delete", params, &result); err != nil {
			return fmt.Errorf("failed to delete findings: %w", err)
		}

		fmt.Printf("Deleted %d finding(s)\n", result.Deleted)
		return nil
	},
}

func init() {
	findingsGetCmd.Flags().StringArrayVarP(&findingsGetKeys, "key", "k", nil, "Key(s) to fetch (can be repeated)")

	findingsCmd.AddCommand(findingsAddCmd)
	findingsCmd.AddCommand(findingsGetCmd)
	findingsCmd.AddCommand(findingsAppendCmd)
	findingsCmd.AddCommand(findingsDeleteCmd)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
