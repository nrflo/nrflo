package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	obsPrjProjectID    string
	obsPrjWorkflowID   string
	obsPrjEnvName      string
	obsPrjEnvValue     string
	obsPrjWFCreateJSON string
)

var projectGroupCmd = &cobra.Command{
	Use:           "project",
	Short:         "Observer project commands",
	Hidden:        true,
	SilenceUsage:  true,
	SilenceErrors: true,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if os.Getenv("NRF_OBSERVER") != "1" {
			return unknownCmdErr("observer", cmd.Root().Name())
		}
		if os.Getenv("NRF_OBSERVER_SCOPE") != "project" {
			return unknownCmdErr("project", cmd.Root().Name())
		}
		return nil
	},
}

func obsPrjBaseParams() map[string]interface{} {
	params := map[string]interface{}{}
	if obsPrjProjectID != "" {
		params["project_id"] = obsPrjProjectID
	}
	return params
}

var obsProjectWorkflowsCmd = &cobra.Command{
	Use:   "workflows",
	Short: "List workflow definitions for a project",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runObserverMethod("observer.project.workflows", obsPrjBaseParams())
	},
}

var obsProjectRunsCmd = &cobra.Command{
	Use:   "runs",
	Short: "List project-scoped workflow instances",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runObserverMethod("observer.project.runs", obsPrjBaseParams())
	},
}

var obsProjectFindingsCmd = &cobra.Command{
	Use:   "findings",
	Short: "Get project findings",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runObserverMethod("observer.project.findings", obsPrjBaseParams())
	},
}

var obsProjectEnvCmd = &cobra.Command{
	Use:           "env",
	Short:         "Project env var subcommands",
	SilenceUsage:  true,
	SilenceErrors: true,
}

var obsProjectEnvListCmd = &cobra.Command{
	Use:   "list",
	Short: "List project env vars",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runObserverMethod("observer.project.env.list", obsPrjBaseParams())
	},
}

var obsProjectEnvSetCmd = &cobra.Command{
	Use:   "set",
	Short: "Upsert a project env var (mutate)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		params := obsPrjBaseParams()
		params["name"] = obsPrjEnvName
		params["value"] = obsPrjEnvValue
		return runObserverMethod("observer.project.env.set", params)
	},
}

var obsProjectEnvUnsetCmd = &cobra.Command{
	Use:   "unset",
	Short: "Delete a project env var (mutate)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		params := obsPrjBaseParams()
		params["name"] = obsPrjEnvName
		return runObserverMethod("observer.project.env.unset", params)
	},
}

var obsProjectWFCmd = &cobra.Command{
	Use:           "workflow",
	Short:         "Project workflow definition subcommands",
	SilenceUsage:  true,
	SilenceErrors: true,
}

var obsProjectWFCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a workflow definition (mutate)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		params := obsPrjBaseParams()
		if obsPrjWFCreateJSON != "" {
			var body map[string]interface{}
			if err := json.Unmarshal([]byte(obsPrjWFCreateJSON), &body); err != nil {
				return fmt.Errorf("invalid JSON: %w", err)
			}
			for k, v := range body {
				params[k] = v
			}
		}
		return runObserverMethod("observer.project.workflow.create", params)
	},
}

var obsProjectWFDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete a workflow definition (mutate)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		params := obsPrjBaseParams()
		if obsPrjWorkflowID != "" {
			params["workflow_id"] = obsPrjWorkflowID
		}
		return runObserverMethod("observer.project.workflow.delete", params)
	},
}

func init() {
	projectGroupCmd.PersistentFlags().StringVar(&obsPrjProjectID, "project-id", "", "Project ID")

	obsProjectEnvSetCmd.Flags().StringVar(&obsPrjEnvName, "name", "", "Env var name")
	obsProjectEnvSetCmd.Flags().StringVar(&obsPrjEnvValue, "value", "", "Env var value")

	obsProjectEnvUnsetCmd.Flags().StringVar(&obsPrjEnvName, "name", "", "Env var name")

	obsProjectWFCreateCmd.Flags().StringVar(&obsPrjWFCreateJSON, "json", "", "JSON body (WorkflowDefCreateRequest)")

	obsProjectWFDeleteCmd.Flags().StringVar(&obsPrjWorkflowID, "workflow-id", "", "Workflow ID")

	obsProjectEnvCmd.AddCommand(obsProjectEnvListCmd, obsProjectEnvSetCmd, obsProjectEnvUnsetCmd)
	obsProjectWFCmd.AddCommand(obsProjectWFCreateCmd, obsProjectWFDeleteCmd)
	projectGroupCmd.AddCommand(
		obsProjectWorkflowsCmd,
		obsProjectRunsCmd,
		obsProjectFindingsCmd,
		obsProjectEnvCmd,
		obsProjectWFCmd,
	)
}
