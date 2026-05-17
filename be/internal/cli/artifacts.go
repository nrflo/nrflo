package cli

import (
	"encoding/base64"
	"fmt"
	"os"
	"sort"

	"github.com/spf13/cobra"
)

var artifactCmd = &cobra.Command{
	Use:   "artifact",
	Short: "Manage artifacts for the current workflow instance",
}

var artifactAddCmd = &cobra.Command{
	Use:   "add <file-path> <NAME>",
	Short: "Upload a file as a named artifact",
	Args:  cobra.ExactArgs(2),
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

		filePath := args[0]
		name := args[1]

		data, err := os.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("read file %s: %w", filePath, err)
		}

		encoded := base64.StdEncoding.EncodeToString(data)
		params := map[string]interface{}{
			"name":        name,
			"content_b64": encoded,
		}
		addSpawnerIDs(params)

		var result struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}
		if err := GetClient().ExecuteAndUnmarshal("artifact.add", params, &result); err != nil {
			return err
		}
		fmt.Printf("id=%s name=%s\n", result.ID, result.Name)
		return nil
	},
}

var artifactListCmd = &cobra.Command{
	Use:   "list",
	Short: "List artifacts for the current workflow instance",
	Args:  cobra.NoArgs,
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

		params := map[string]interface{}{}
		addSpawnerIDs(params)

		var result []struct {
			Name        string `json:"name"`
			SizeBytes   int64  `json:"size_bytes"`
			ContentType string `json:"content_type"`
			Source      string `json:"source"`
		}
		if err := GetClient().ExecuteAndUnmarshal("artifact.list", params, &result); err != nil {
			return err
		}

		sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
		for _, a := range result {
			fmt.Printf("%s\t%d\t%s\n", a.Name, a.SizeBytes, a.Source)
		}
		return nil
	},
}

var artifactGetCmd = &cobra.Command{
	Use:   "get <NAME>",
	Short: "Get the local path to a materialized artifact",
	Args:  cobra.ExactArgs(1),
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

		name := args[0]
		params := map[string]interface{}{"name": name}
		addSpawnerIDs(params)

		var result struct {
			Path string `json:"path"`
		}
		if err := GetClient().ExecuteAndUnmarshal("artifact.get", params, &result); err != nil {
			return err
		}
		fmt.Print(result.Path)
		return nil
	},
}

func init() {
	artifactCmd.AddCommand(artifactAddCmd)
	artifactCmd.AddCommand(artifactListCmd)
	artifactCmd.AddCommand(artifactGetCmd)
}
