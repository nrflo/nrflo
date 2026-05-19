package cli

import (
	"os"

	"github.com/spf13/cobra"
)

var (
	obsGblProjectID       string
	obsGblLimit           int
	obsGblProjectName     string
	obsGblProjectRootPath string
	obsGblDefaultBranch   string
)

var globalGroupCmd = &cobra.Command{
	Use:           "global",
	Short:         "Observer global commands",
	Hidden:        true,
	SilenceUsage:  true,
	SilenceErrors: true,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if os.Getenv("NRF_OBSERVER") != "1" {
			return unknownCmdErr("observer", cmd.Root().Name())
		}
		if os.Getenv("NRF_OBSERVER_SCOPE") != "global" {
			return unknownCmdErr("global", cmd.Root().Name())
		}
		return nil
	},
}

var obsGlobalProjectsCmd = &cobra.Command{
	Use:   "projects",
	Short: "List all projects",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runObserverMethod("observer.global.projects", map[string]interface{}{})
	},
}

var obsGlobalRecentSessionsCmd = &cobra.Command{
	Use:   "recent-sessions",
	Short: "List recent agent sessions",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		params := map[string]interface{}{}
		if obsGblProjectID != "" {
			params["project_id"] = obsGblProjectID
		}
		if obsGblLimit > 0 {
			params["limit"] = obsGblLimit
		}
		return runObserverMethod("observer.global.recent_sessions", params)
	},
}

var obsGlobalHealthCmd = &cobra.Command{
	Use:   "health",
	Short: "DB ping and observer feature flag status",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runObserverMethod("observer.global.health", map[string]interface{}{})
	},
}

var obsGlobalProjectCmd = &cobra.Command{
	Use:           "project",
	Short:         "Global project subcommands",
	SilenceUsage:  true,
	SilenceErrors: true,
}

var obsGlobalProjectCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a project (mutate)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		params := map[string]interface{}{}
		if obsGblProjectID != "" {
			params["project_id"] = obsGblProjectID
		}
		if obsGblProjectName != "" {
			params["name"] = obsGblProjectName
		}
		if obsGblProjectRootPath != "" {
			params["root_path"] = obsGblProjectRootPath
		}
		if obsGblDefaultBranch != "" {
			params["default_branch"] = obsGblDefaultBranch
		}
		return runObserverMethod("observer.global.project.create", params)
	},
}

var obsGlobalProjectDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete a project (mutate)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		params := map[string]interface{}{}
		if obsGblProjectID != "" {
			params["project_id"] = obsGblProjectID
		}
		return runObserverMethod("observer.global.project.delete", params)
	},
}

func init() {
	obsGlobalRecentSessionsCmd.Flags().StringVar(&obsGblProjectID, "project-id", "", "Project ID")
	obsGlobalRecentSessionsCmd.Flags().IntVar(&obsGblLimit, "limit", 0, "Max sessions to return")

	obsGlobalProjectCreateCmd.Flags().StringVar(&obsGblProjectID, "project-id", "", "Project ID (required)")
	obsGlobalProjectCreateCmd.Flags().StringVar(&obsGblProjectName, "name", "", "Project name")
	obsGlobalProjectCreateCmd.Flags().StringVar(&obsGblProjectRootPath, "root-path", "", "Project root path")
	obsGlobalProjectCreateCmd.Flags().StringVar(&obsGblDefaultBranch, "default-branch", "", "Default branch")

	obsGlobalProjectDeleteCmd.Flags().StringVar(&obsGblProjectID, "project-id", "", "Project ID (required)")

	obsGlobalProjectCmd.AddCommand(obsGlobalProjectCreateCmd, obsGlobalProjectDeleteCmd)
	globalGroupCmd.AddCommand(
		obsGlobalProjectsCmd,
		obsGlobalRecentSessionsCmd,
		obsGlobalHealthCmd,
		obsGlobalProjectCmd,
	)
}
