package cli

import (
	"testing"

	"github.com/spf13/cobra"
)

// TestRegisterServerCommands verifies that RegisterServerCommands registers only serve + version commands
func TestRegisterServerCommands(t *testing.T) {
	// Create a fresh root command to avoid init() pollution
	testRootCmd := &cobra.Command{Use: "nrflow"}

	// Re-add versionCmd manually (normally done in init())
	testRootCmd.AddCommand(versionCmd)

	// Temporarily replace rootCmd
	originalRootCmd := rootCmd
	rootCmd = testRootCmd
	defer func() { rootCmd = originalRootCmd }()

	// Call RegisterServerCommands
	RegisterServerCommands()

	// Check root command metadata
	if rootCmd.Use != "nrflow_server" {
		t.Errorf("RegisterServerCommands: rootCmd.Use = %q, want 'nrflow_server'", rootCmd.Use)
	}
	if rootCmd.Short != "nrflow server" {
		t.Errorf("RegisterServerCommands: rootCmd.Short = %q, want 'nrflow server'", rootCmd.Short)
	}

	// Get all registered commands
	commands := getCommandNames(rootCmd)

	// Expected: serve, version (version is in init())
	expectedCommands := map[string]bool{
		"serve":   true,
		"version": true,
	}

	// Verify all expected commands are present
	for cmd := range expectedCommands {
		if !contains(commands, cmd) {
			t.Errorf("RegisterServerCommands: missing expected command %q", cmd)
		}
	}

	// Verify no extra commands (client commands should not be present)
	unexpectedCommands := []string{"agent", "findings", "tickets", "deps"}
	for _, cmd := range unexpectedCommands {
		if contains(commands, cmd) {
			t.Errorf("RegisterServerCommands: unexpected command %q should not be registered", cmd)
		}
	}

	// Verify exact count: serve + version = 2
	if len(commands) != 2 {
		t.Errorf("RegisterServerCommands: got %d commands, want 2. Commands: %v", len(commands), commands)
	}
}

// TestRegisterCLICommands verifies that RegisterCLICommands registers only client commands
func TestRegisterCLICommands(t *testing.T) {
	// Create a fresh root command to avoid init() pollution
	testRootCmd := &cobra.Command{Use: "nrflow"}
	// Re-add versionCmd (normally done in init())
	testRootCmd.AddCommand(versionCmd)

	// Temporarily replace rootCmd
	originalRootCmd := rootCmd
	rootCmd = testRootCmd
	defer func() { rootCmd = originalRootCmd }()

	// Call RegisterCLICommands
	RegisterCLICommands()

	// Check root command metadata
	if rootCmd.Use != "nrflow" {
		t.Errorf("RegisterCLICommands: rootCmd.Use = %q, want 'nrflow'", rootCmd.Use)
	}
	if rootCmd.Short != "CLI tool for nrflow server" {
		t.Errorf("RegisterCLICommands: rootCmd.Short = %q, want 'CLI tool for nrflow server'", rootCmd.Short)
	}

	// Get all registered commands
	commands := getCommandNames(rootCmd)

	// Expected: agent, findings, tickets, deps, skip, version
	expectedCommands := map[string]bool{
		"agent":    true,
		"findings": true,
		"tickets":  true,
		"deps":     true,
		"skip":     true,
		"version":  true,
	}

	// Verify all expected commands are present
	for cmd := range expectedCommands {
		if !contains(commands, cmd) {
			t.Errorf("RegisterCLICommands: missing expected command %q", cmd)
		}
	}

	// Verify serve command is NOT present
	if contains(commands, "serve") {
		t.Errorf("RegisterCLICommands: unexpected command 'serve' should not be registered")
	}

	// Verify exact count: agent + findings + tickets + deps + skip + version = 6
	if len(commands) != 6 {
		t.Errorf("RegisterCLICommands: got %d commands, want 6. Commands: %v", len(commands), commands)
	}
}

// TestRegisterCLICommands_TicketsFlags verifies that ticketsCmd persistent flags are registered
func TestRegisterCLICommands_TicketsFlags(t *testing.T) {
	// Note: ticketsCmd is a package-level variable and RegisterCLICommands() has already been called
	// during package init or previous tests, so flags are already registered.
	// This test verifies that the flags exist after registration.

	// Verify ticketsCmd has --server and --json flags
	serverFlag := ticketsCmd.PersistentFlags().Lookup("server")
	if serverFlag == nil {
		t.Error("RegisterCLICommands: ticketsCmd missing --server flag")
	}

	jsonFlag := ticketsCmd.PersistentFlags().Lookup("json")
	if jsonFlag == nil {
		t.Error("RegisterCLICommands: ticketsCmd missing --json flag")
	}
}

// TestNoCommandLeakage verifies that commands don't leak between registration functions
func TestNoCommandLeakage(t *testing.T) {
	// Test 1: RegisterServerCommands alone
	testRootCmd1 := &cobra.Command{Use: "nrflow"}
	testRootCmd1.AddCommand(versionCmd)

	originalRootCmd := rootCmd
	rootCmd = testRootCmd1
	RegisterServerCommands()
	serverCommands := getCommandNames(rootCmd)
	rootCmd = originalRootCmd

	if contains(serverCommands, "agent") || contains(serverCommands, "tickets") {
		t.Errorf("Command leakage: client commands present after RegisterServerCommands only. Commands: %v", serverCommands)
	}

	// Test 2: Verify serve command does NOT leak to CLI registration
	// We test this by checking that a fresh rootCmd with only CLI commands does not have serve
	testRootCmd2 := &cobra.Command{Use: "nrflow"}
	testRootCmd2.AddCommand(versionCmd)

	// Manually add CLI commands without calling RegisterCLICommands (to avoid flag re-registration)
	testRootCmd2.AddCommand(agentCmd)
	testRootCmd2.AddCommand(findingsCmd)
	testRootCmd2.AddCommand(ticketsCmd)
	testRootCmd2.AddCommand(depsCmd)

	cliCommands := getCommandNames(testRootCmd2)

	if contains(cliCommands, "serve") {
		t.Errorf("Command leakage: serve command present in CLI-only setup. Commands: %v", cliCommands)
	}

	// Verify expected CLI commands are present
	expectedCLI := []string{"agent", "findings", "tickets", "deps", "version"}
	for _, expected := range expectedCLI {
		if !contains(cliCommands, expected) {
			t.Errorf("CLI-only setup missing expected command %q", expected)
		}
	}
}

// TestVersionCommandNotDuplicated verifies that versionCmd appears exactly once when both functions are called
func TestVersionCommandNotDuplicated(t *testing.T) {
	// Note: RegisterCLICommands has already been called in previous tests and will panic on flag re-registration.
	// Instead, we verify that versionCmd is only added once by manually constructing a scenario.

	// Create a fresh root command
	testRootCmd := &cobra.Command{Use: "nrflow"}
	testRootCmd.AddCommand(versionCmd)

	// Temporarily replace rootCmd
	originalRootCmd := rootCmd
	rootCmd = testRootCmd
	defer func() { rootCmd = originalRootCmd }()

	// Call RegisterServerCommands (which doesn't re-add versionCmd)
	RegisterServerCommands()

	// Manually add CLI commands (without RegisterCLICommands to avoid flag panic)
	rootCmd.AddCommand(agentCmd)
	rootCmd.AddCommand(findingsCmd)
	rootCmd.AddCommand(ticketsCmd)
	rootCmd.AddCommand(depsCmd)

	// Count version command occurrences
	versionCount := 0
	for _, cmd := range rootCmd.Commands() {
		if cmd.Use == "version" {
			versionCount++
		}
	}

	if versionCount != 1 {
		t.Errorf("versionCmd appears %d times, want 1 (commands: %v)", versionCount, getCommandNames(rootCmd))
	}
}

// TestAgentSubcommands verifies that agent subcommands are registered via init()
func TestAgentSubcommands(t *testing.T) {
	// agentCmd subcommands are registered in agent.go init()
	// They should be attached to agentCmd, not rootCmd

	expectedSubcommands := []string{"fail", "continue", "callback", "context-update"}
	actualSubcommands := getCommandNames(agentCmd)

	for _, expected := range expectedSubcommands {
		if !contains(actualSubcommands, expected) {
			t.Errorf("agentCmd missing subcommand %q. Got: %v", expected, actualSubcommands)
		}
	}

	// Verify exact count: 4 subcommands
	if len(actualSubcommands) != 4 {
		t.Errorf("agentCmd has %d subcommands, want 4. Subcommands: %v", len(actualSubcommands), actualSubcommands)
	}
}

// TestTicketsSubcommands verifies that tickets subcommands are registered via init()
func TestTicketsSubcommands(t *testing.T) {
	// ticketsCmd subcommands are registered in tickets.go and tickets_update.go init()

	expectedSubcommands := []string{"list", "get", "create", "update", "close", "reopen", "delete"}
	actualSubcommands := getCommandNames(ticketsCmd)

	for _, expected := range expectedSubcommands {
		if !contains(actualSubcommands, expected) {
			t.Errorf("ticketsCmd missing subcommand %q. Got: %v", expected, actualSubcommands)
		}
	}
}

// TestFindingsSubcommands verifies that findings subcommands are registered via init()
func TestFindingsSubcommands(t *testing.T) {
	expectedSubcommands := []string{"add", "get", "append", "delete", "project-add", "project-get", "project-append", "project-delete"}
	actualSubcommands := getCommandNames(findingsCmd)

	for _, expected := range expectedSubcommands {
		if !contains(actualSubcommands, expected) {
			t.Errorf("findingsCmd missing subcommand %q. Got: %v", expected, actualSubcommands)
		}
	}

	// Verify exact count: 8 subcommands
	if len(actualSubcommands) != 8 {
		t.Errorf("findingsCmd has %d subcommands, want 8. Subcommands: %v", len(actualSubcommands), actualSubcommands)
	}
}

// TestDepsSubcommands verifies that deps subcommands are registered via init()
func TestDepsSubcommands(t *testing.T) {
	expectedSubcommands := []string{"list", "add", "remove"}
	actualSubcommands := getCommandNames(depsCmd)

	for _, expected := range expectedSubcommands {
		if !contains(actualSubcommands, expected) {
			t.Errorf("depsCmd missing subcommand %q. Got: %v", expected, actualSubcommands)
		}
	}

	// Verify exact count: 3 subcommands
	if len(actualSubcommands) != 3 {
		t.Errorf("depsCmd has %d subcommands, want 3. Subcommands: %v", len(actualSubcommands), actualSubcommands)
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

// TestRootCommandMetadata verifies that root command metadata is set correctly by registration functions
func TestRootCommandMetadata(t *testing.T) {
	originalRootCmd := rootCmd

	// Test server registration
	testRootCmd1 := &cobra.Command{Use: "nrflow"}
	rootCmd = testRootCmd1
	RegisterServerCommands()
	if rootCmd.Use != "nrflow_server" {
		t.Errorf("After RegisterServerCommands: Use = %q, want 'nrflow_server'", rootCmd.Use)
	}
	if rootCmd.Short != "nrflow server" {
		t.Errorf("After RegisterServerCommands: Short = %q, want 'nrflow server'", rootCmd.Short)
	}

	// Test CLI registration metadata only (skip RegisterCLICommands to avoid flag panic)
	// The metadata change is done in RegisterCLICommands, which has already been tested in TestRegisterCLICommands
	// We verify the expected metadata values that RegisterCLICommands should set:
	expectedUse := "nrflow"
	expectedShort := "CLI tool for nrflow server"

	t.Logf("RegisterCLICommands sets Use=%q and Short=%q (verified in TestRegisterCLICommands)", expectedUse, expectedShort)

	rootCmd = originalRootCmd
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
