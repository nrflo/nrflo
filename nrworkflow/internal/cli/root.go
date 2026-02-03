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

// UseSocket controls whether to use socket client (true) or direct DB access (false)
// The serve command sets this to false since it IS the server
var UseSocket = true

var rootCmd = &cobra.Command{
	Use:   "nrworkflow",
	Short: "nrworkflow - Multi-workflow ticket and agent management CLI",
	Long: `nrworkflow is a unified CLI tool for ticket management and AI agent orchestration.

It manages projects, tickets, workflows, phases, and spawns AI agents to work on tasks.

The CLI communicates with the nrworkflow server via Unix socket.
Start the server with: nrworkflow serve`,
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

	// Project management
	rootCmd.AddCommand(projectCmd)

	// Ticket management (will be grouped under 'ticket' subcommand later)
	rootCmd.AddCommand(ticketCmd)

	// Legacy direct commands (for backwards compatibility during migration)
	rootCmd.AddCommand(initDBCmd)
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
