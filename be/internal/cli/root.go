package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"be/internal/client"
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
	Short: "nrworkflow - Multi-workflow agent orchestration",
	Long: `nrworkflow is the agent CLI for nrworkflow orchestration system.

Agent commands (used by spawned agents):
  nrworkflow agent fail/continue <ticket> <agent-type> -w <workflow>
  nrworkflow findings add/append/get/delete ...`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if envProject := os.Getenv("NRWORKFLOW_PROJECT"); envProject != "" {
			ProjectID = envProject
		}
		return nil
	},
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&DataPath, "data", "D", "", "Path to database file")
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
		return fmt.Errorf("project not found. Set NRWORKFLOW_PROJECT env variable")
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

// GetWorkflowInstanceID returns the workflow instance ID from env var (set by spawner)
func GetWorkflowInstanceID() string {
	return os.Getenv("NRWF_WORKFLOW_INSTANCE_ID")
}

// GetSessionID returns the agent session ID from env var (set by spawner)
func GetSessionID() string {
	return os.Getenv("NRWF_SESSION_ID")
}

// addSpawnerIDs adds instance_id and session_id to socket params from env vars
func addSpawnerIDs(params map[string]interface{}) {
	if id := GetWorkflowInstanceID(); id != "" {
		params["instance_id"] = id
	}
	if id := GetSessionID(); id != "" {
		params["session_id"] = id
	}
}
