package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"nrworkflow/internal/client"
	"nrworkflow/internal/config"
)

var version = "1.0.0"

// DataPath holds the custom data file path (from --data flag)
var DataPath string

// ProjectID holds the current project ID (from NRWORKFLOW_PROJECT env or .claude/nrworkflow/config.json)
var ProjectID string

// ProjectRoot holds the root directory of the project (where .claude/nrworkflow/config.json was found)
var ProjectRoot string

var rootCmd = &cobra.Command{
	Use:   "nrworkflow",
	Short: "nrworkflow - Multi-workflow agent orchestration server",
	Long: `nrworkflow is a server for ticket management and AI agent orchestration.

Start the server with: nrworkflow serve
Manage workflows and tickets via the web UI at http://localhost:6587

Agent CLI subset (used by spawned agents):
  nrworkflow agent complete/fail/continue <ticket> <agent-type> -w <workflow>
  nrworkflow findings add/append/get/delete ...`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Priority 1: Environment variable (for CI/CD, scripting)
		if envProject := os.Getenv("NRWORKFLOW_PROJECT"); envProject != "" {
			ProjectID = envProject
		}

		// Priority 2: Search for .claude/nrworkflow/config.json upward
		if ProjectID == "" {
			result, err := config.FindProjectConfig("")
			if err != nil {
				// Only warn, don't fail - some commands don't need a project
				fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
			} else if result != nil && result.Config != nil && result.Config.Project != "" {
				ProjectID = result.Config.Project
				ProjectRoot = result.ConfigDir
			}
		}

		return nil
	},
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&DataPath, "data", "D", "", "Path to database file")

	rootCmd.AddCommand(serveCmd)
	rootCmd.AddCommand(versionCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number",
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Printf("nrworkflow version %s\n", version)
	},
}

// Execute runs the root command
func Execute() error {
	return rootCmd.Execute()
}

// GetClient returns a socket client for CLI commands
func GetClient() *client.Client {
	return client.New(ProjectID)
}

// CheckServer checks if the server is running and returns an error if not
func CheckServer() error {
	c := GetClient()
	if !c.IsServerRunning() {
		return client.ServerNotRunningError()
	}
	return nil
}

// RequireProject is a helper that ensures ProjectID is set
func RequireProject() error {
	if ProjectID == "" {
		return fmt.Errorf("project not found. Create .claude/nrworkflow/config.json with 'project' field, or set NRWORKFLOW_PROJECT env")
	}
	return nil
}

// GetProjectRootPath returns the root path for the current project
func GetProjectRootPath() string {
	if ProjectRoot != "" {
		return ProjectRoot
	}
	return "."
}
