package cli

import (
	"os"

	"github.com/spf13/cobra"

	"be/internal/client"
)

var version = "1.0.0"

// DataPath holds the custom data file path (from --data flag)
var DataPath string

// ProjectID holds the current project ID (from NRFLO_PROJECT env or .claude/nrflo/config.json)
var ProjectID string

// ProjectRoot holds the root directory of the project (where .claude/nrflo/config.json was found)
var ProjectRoot string

var rootCmd = &cobra.Command{
	Use:   "nrflo_server",
	Short: "nrflo - Multi-workflow agent orchestration server",
	Long: `nrflo_server is the nrflo orchestration server.

It serves the HTTP API + WebSocket for the web UI and a Unix socket for agent
communication, and hosts the agent infrastructure subcommands (the MCP tool
bridge and Claude hook/statusline forwarders) the spawner invokes.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if envProject := os.Getenv("NRFLO_PROJECT"); envProject != "" {
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
		cmd.Printf("nrflo version %s\n", version)
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

// GetWorkflowInstanceID returns the workflow instance ID from env var (set by spawner)
func GetWorkflowInstanceID() string {
	return os.Getenv("NRF_WORKFLOW_INSTANCE_ID")
}

// GetSessionID returns the agent session ID from env var (set by spawner)
func GetSessionID() string {
	return os.Getenv("NRF_SESSION_ID")
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
