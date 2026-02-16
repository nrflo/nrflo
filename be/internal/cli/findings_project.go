package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"be/internal/client"
)

var projFindingsGetKeys []string

var projFindingsAddCmd = &cobra.Command{
	Use:   "project-add <key> <value> | <key:value>...",
	Short: "Add project-level finding(s)",
	Long: `Add one or more project-level findings.

Two syntax modes:
  1. Single finding: <key> <value> as separate arguments
  2. Multiple findings: key:value pairs

Examples:
  nrworkflow findings project-add mykey myvalue
  nrworkflow findings project-add k1:v1 k2:v2`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := RequireProject(); err != nil {
			return err
		}
		if err := CheckServer(); err != nil {
			return err
		}

		c := GetClient()

		// Single finding: exactly 2 args and first has no colon
		if len(args) == 2 && !strings.Contains(args[0], ":") {
			params := map[string]interface{}{
				"key":   args[0],
				"value": args[1],
			}
			if err := c.ExecuteAndUnmarshal("project_findings.add", params, nil); err != nil {
				return fmt.Errorf("failed to add project finding: %w", err)
			}
			fmt.Printf("Added project finding: %s = %s\n", args[0], truncate(args[1], 50))
			return nil
		}

		// Bulk mode: key:value pairs
		keyValues := make(map[string]string)
		for _, arg := range args {
			kv, err := parseKeyValue(arg)
			if err != nil {
				return fmt.Errorf("invalid key:value format '%s': %w", arg, err)
			}
			keyValues[kv.key] = kv.value
		}

		var result struct {
			Status string `json:"status"`
			Count  int    `json:"count"`
		}
		if err := c.ExecuteAndUnmarshal("project_findings.add-bulk", map[string]interface{}{
			"key_values": keyValues,
		}, &result); err != nil {
			return fmt.Errorf("failed to add project findings: %w", err)
		}

		fmt.Printf("Added %d project finding(s)\n", result.Count)
		return nil
	},
}

var projFindingsGetCmd = &cobra.Command{
	Use:   "project-get [key]",
	Short: "Get project-level findings",
	Long: `Get project-level findings.

If no key is specified, returns all project findings.
Use -k/--key to fetch specific keys (can be repeated).

Examples:
  nrworkflow findings project-get
  nrworkflow findings project-get mykey
  nrworkflow findings project-get -k k1 -k k2`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := RequireProject(); err != nil {
			return err
		}
		if err := CheckServer(); err != nil {
			return err
		}

		// Collect keys: from positional arg and/or -k flags
		var keys []string
		if len(args) > 0 {
			keys = append(keys, args[0])
		}
		keys = append(keys, projFindingsGetKeys...)

		c := GetClient()
		params := map[string]interface{}{}

		if len(keys) == 1 {
			params["key"] = keys[0]
		} else if len(keys) > 1 {
			params["keys"] = keys
		}

		var result interface{}
		if err := c.ExecuteAndUnmarshal("project_findings.get", params, &result); err != nil {
			return err
		}

		fmt.Println(client.FormatValue(result))
		return nil
	},
}

var projFindingsAppendCmd = &cobra.Command{
	Use:   "project-append <key> <value> | <key:value>...",
	Short: "Append to project-level finding(s)",
	Long: `Append one or more values to project-level findings.

Two syntax modes:
  1. Single append: <key> <value> as separate arguments
  2. Multiple appends: key:value pairs

Examples:
  nrworkflow findings project-append mykey newval
  nrworkflow findings project-append k1:v1 k2:v2`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := RequireProject(); err != nil {
			return err
		}
		if err := CheckServer(); err != nil {
			return err
		}

		c := GetClient()

		// Single append: exactly 2 args and first has no colon
		if len(args) == 2 && !strings.Contains(args[0], ":") {
			params := map[string]interface{}{
				"key":   args[0],
				"value": args[1],
			}
			if err := c.ExecuteAndUnmarshal("project_findings.append", params, nil); err != nil {
				return fmt.Errorf("failed to append project finding: %w", err)
			}
			fmt.Printf("Appended to project finding: %s\n", args[0])
			return nil
		}

		// Bulk mode: key:value pairs
		keyValues := make(map[string]string)
		for _, arg := range args {
			kv, err := parseKeyValue(arg)
			if err != nil {
				return fmt.Errorf("invalid key:value format '%s': %w", arg, err)
			}
			keyValues[kv.key] = kv.value
		}

		var result struct {
			Status string `json:"status"`
			Count  int    `json:"count"`
		}
		if err := c.ExecuteAndUnmarshal("project_findings.append-bulk", map[string]interface{}{
			"key_values": keyValues,
		}, &result); err != nil {
			return fmt.Errorf("failed to append project findings: %w", err)
		}

		fmt.Printf("Appended to %d project finding(s)\n", result.Count)
		return nil
	},
}

var projFindingsDeleteCmd = &cobra.Command{
	Use:   "project-delete <key>...",
	Short: "Delete project-level finding key(s)",
	Long: `Delete one or more project-level finding keys.

Examples:
  nrworkflow findings project-delete mykey
  nrworkflow findings project-delete k1 k2 k3`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := RequireProject(); err != nil {
			return err
		}
		if err := CheckServer(); err != nil {
			return err
		}

		c := GetClient()

		var result struct {
			Status  string `json:"status"`
			Deleted int    `json:"deleted"`
		}
		if err := c.ExecuteAndUnmarshal("project_findings.delete", map[string]interface{}{
			"keys": args,
		}, &result); err != nil {
			return fmt.Errorf("failed to delete project findings: %w", err)
		}

		fmt.Printf("Deleted %d project finding(s)\n", result.Deleted)
		return nil
	},
}

func init() {
	projFindingsGetCmd.Flags().StringArrayVarP(&projFindingsGetKeys, "key", "k", nil, "Key(s) to fetch (can be repeated)")

	findingsCmd.AddCommand(projFindingsAddCmd)
	findingsCmd.AddCommand(projFindingsGetCmd)
	findingsCmd.AddCommand(projFindingsAppendCmd)
	findingsCmd.AddCommand(projFindingsDeleteCmd)
}
