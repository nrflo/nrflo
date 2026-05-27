package cli

import (
	"testing"

	"github.com/spf13/cobra"
)

// TestRegisterServerCommands verifies that RegisterServerCommands registers serve + version + the
// agent infrastructure parent (record-event/statusline/context-update/mcp), and no client commands.
func TestRegisterServerCommands(t *testing.T) {
	// Create a fresh root command to avoid init() pollution
	testRootCmd := &cobra.Command{Use: "nrflo"}

	// Re-add versionCmd manually (normally done in init())
	testRootCmd.AddCommand(versionCmd)

	// Temporarily replace rootCmd
	originalRootCmd := rootCmd
	rootCmd = testRootCmd
	defer func() { rootCmd = originalRootCmd }()

	// Call RegisterServerCommands
	RegisterServerCommands()

	// Check root command metadata
	if rootCmd.Use != "nrflo_server" {
		t.Errorf("RegisterServerCommands: rootCmd.Use = %q, want 'nrflo_server'", rootCmd.Use)
	}
	if rootCmd.Short != "nrflo server" {
		t.Errorf("RegisterServerCommands: rootCmd.Short = %q, want 'nrflo server'", rootCmd.Short)
	}

	// Get all registered commands
	commands := getCommandNames(rootCmd)

	// Expected: serve, version (init()), agent (infra parent)
	expectedCommands := map[string]bool{
		"serve":   true,
		"version": true,
		"agent":   true,
	}

	// Verify all expected commands are present
	for cmd := range expectedCommands {
		if !contains(commands, cmd) {
			t.Errorf("RegisterServerCommands: missing expected command %q", cmd)
		}
	}

	// Verify no client commands present (agent here is the infra parent, not the CLI's agent).
	unexpectedCommands := []string{"findings", "tickets", "deps", "observer"}
	for _, cmd := range unexpectedCommands {
		if contains(commands, cmd) {
			t.Errorf("RegisterServerCommands: unexpected command %q should not be registered", cmd)
		}
	}

	// Verify exact count: serve + version + agent = 3
	if len(commands) != 3 {
		t.Errorf("RegisterServerCommands: got %d commands, want 3. Commands: %v", len(commands), commands)
	}

	// The agent infra parent must expose exactly the infrastructure subcommands.
	var agentCommands []string
	for _, c := range rootCmd.Commands() {
		if c.Name() == "agent" {
			agentCommands = getCommandNames(c)
		}
	}
	for _, sub := range []string{"record-event", "statusline", "context-update", "mcp"} {
		if !contains(agentCommands, sub) {
			t.Errorf("RegisterServerCommands: agent missing infra subcommand %q (have %v)", sub, agentCommands)
		}
	}
	// Agent-initiated commands must NOT be on the server.
	for _, sub := range []string{"finished", "fail", "callback", "continue"} {
		if contains(agentCommands, sub) {
			t.Errorf("RegisterServerCommands: agent should not expose agent-initiated subcommand %q", sub)
		}
	}
}

// TestDataPathFlagRegistered verifies that DataPath flag is registered in root init()
func TestDataPathFlagRegistered(t *testing.T) {
	// DataPath should be registered in root.go init() regardless of which registration function is called
	dataFlag := rootCmd.PersistentFlags().Lookup("data")
	if dataFlag == nil {
		t.Error("rootCmd missing --data flag (should be registered in init())")
	}
}

// TestRootCommandMetadata verifies that RegisterServerCommands sets the server metadata.
func TestRootCommandMetadata(t *testing.T) {
	originalRootCmd := rootCmd
	defer func() { rootCmd = originalRootCmd }()

	rootCmd = &cobra.Command{Use: "nrflo"}
	RegisterServerCommands()
	if rootCmd.Use != "nrflo_server" {
		t.Errorf("After RegisterServerCommands: Use = %q, want 'nrflo_server'", rootCmd.Use)
	}
	if rootCmd.Short != "nrflo server" {
		t.Errorf("After RegisterServerCommands: Short = %q, want 'nrflo server'", rootCmd.Short)
	}
}

// Helper functions

// getCommandNames returns a slice of command names from a cobra command
func getCommandNames(cmd *cobra.Command) []string {
	var names []string
	for _, c := range cmd.Commands() {
		// c.Use may include arguments like "callback <ticket> <agent-type>"
		// Extract just the command name (first word)
		name := c.Name()
		names = append(names, name)
	}
	return names
}

// contains checks if a slice contains a string
func contains(slice []string, str string) bool {
	for _, s := range slice {
		if s == str {
			return true
		}
	}
	return false
}
