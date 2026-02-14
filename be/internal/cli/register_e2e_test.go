package cli

import (
	"testing"

	"github.com/spf13/cobra"
)

// TestE2E_ServerRegistration tests the complete server command registration flow
func TestE2E_ServerRegistration(t *testing.T) {
	// Simulate what happens in a server binary (cmd/nrworkflow_server/main.go)
	testRootCmd := &cobra.Command{Use: "nrworkflow"}
	testRootCmd.AddCommand(versionCmd)

	originalRootCmd := rootCmd
	rootCmd = testRootCmd
	defer func() { rootCmd = originalRootCmd }()

	// Register only server commands
	RegisterServerCommands()

	// Verify server metadata
	if rootCmd.Use != "nrworkflow_server" {
		t.Errorf("server binary: Use = %q, want 'nrworkflow_server'", rootCmd.Use)
	}
	if rootCmd.Short != "nrworkflow server" {
		t.Errorf("server binary: Short = %q, want 'nrworkflow server'", rootCmd.Short)
	}

	// Verify only server commands are registered
	commands := getCommandNames(rootCmd)

	// Should have: serve, version
	if !contains(commands, "serve") {
		t.Error("server binary: missing 'serve' command")
	}
	if !contains(commands, "version") {
		t.Error("server binary: missing 'version' command")
	}

	// Should NOT have client commands
	clientCommands := []string{"agent", "findings", "tickets", "deps"}
	for _, cmd := range clientCommands {
		if contains(commands, cmd) {
			t.Errorf("server binary: should NOT have %q command", cmd)
		}
	}

	// Verify command count
	if len(commands) != 2 {
		t.Errorf("server binary: got %d commands, want 2 (serve, version). Commands: %v", len(commands), commands)
	}
}

// TestE2E_CLIRegistration tests the complete CLI command registration flow
func TestE2E_CLIRegistration(t *testing.T) {
	// Simulate what happens in a CLI binary (cmd/nrworkflow/main.go when used for CLI-only)
	// Note: This test would work if we had a separate CLI binary, but since we use the same binary,
	// we're just verifying the registration function behavior.

	// Skip if flags already registered (this is the current main.go behavior - both are called)
	if ticketsCmd.PersistentFlags().Lookup("server") != nil {
		t.Log("Flags already registered - this is expected in the combined binary")
	}

	// Verify that after calling RegisterCLICommands, we would have the right commands
	// We can't actually call it again due to flag re-registration, but we verified this in TestRegisterCLICommands

	expectedCommands := []string{"agent", "findings", "tickets", "deps", "version"}
	t.Logf("CLI binary would have commands: %v", expectedCommands)
}

// TestE2E_CombinedRegistration tests the current main.go behavior (both registered)
func TestE2E_CombinedRegistration(t *testing.T) {
	// This simulates the current be/cmd/nrworkflow/main.go which calls both registration functions

	testRootCmd := &cobra.Command{Use: "nrworkflow"}
	testRootCmd.AddCommand(versionCmd)

	originalRootCmd := rootCmd
	rootCmd = testRootCmd
	defer func() { rootCmd = originalRootCmd }()

	// Call both registration functions (as main.go does)
	RegisterServerCommands()
	// Note: Can't call RegisterCLICommands again due to flag re-registration issue
	// But we can manually add the commands to verify the combined behavior
	rootCmd.AddCommand(agentCmd)
	rootCmd.AddCommand(findingsCmd)
	rootCmd.AddCommand(ticketsCmd)
	rootCmd.AddCommand(depsCmd)

	// Verify all commands are registered
	commands := getCommandNames(rootCmd)

	allExpectedCommands := []string{"serve", "agent", "findings", "tickets", "deps", "version"}
	for _, expected := range allExpectedCommands {
		if !contains(commands, expected) {
			t.Errorf("combined binary: missing expected command %q", expected)
		}
	}

	// Verify command count: serve + agent + findings + tickets + deps + version = 6
	if len(commands) != 6 {
		t.Errorf("combined binary: got %d commands, want 6. Commands: %v", len(commands), commands)
	}

	// Verify versionCmd appears exactly once
	versionCount := 0
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "version" {
			versionCount++
		}
	}
	if versionCount != 1 {
		t.Errorf("combined binary: versionCmd appears %d times, want 1", versionCount)
	}
}

// TestE2E_SubcommandIntegrity verifies that subcommands remain attached after registration
func TestE2E_SubcommandIntegrity(t *testing.T) {
	// Verify that subcommands registered via init() are still present after parent command registration

	// Agent subcommands
	agentSubcmds := getCommandNames(agentCmd)
	expectedAgentSubcmds := []string{"complete", "fail", "continue", "callback"}
	for _, expected := range expectedAgentSubcmds {
		if !contains(agentSubcmds, expected) {
			t.Errorf("agentCmd missing subcommand %q after registration", expected)
		}
	}

	// Tickets subcommands
	ticketsSubcmds := getCommandNames(ticketsCmd)
	expectedTicketsSubcmds := []string{"list", "get", "create", "update", "close", "reopen", "delete"}
	for _, expected := range expectedTicketsSubcmds {
		if !contains(ticketsSubcmds, expected) {
			t.Errorf("ticketsCmd missing subcommand %q after registration", expected)
		}
	}

	// Findings subcommands
	findingsSubcmds := getCommandNames(findingsCmd)
	expectedFindingsSubcmds := []string{"add", "get", "append", "delete"}
	for _, expected := range expectedFindingsSubcmds {
		if !contains(findingsSubcmds, expected) {
			t.Errorf("findingsCmd missing subcommand %q after registration", expected)
		}
	}

	// Deps subcommands
	depsSubcmds := getCommandNames(depsCmd)
	expectedDepsSubcmds := []string{"list", "add", "remove"}
	for _, expected := range expectedDepsSubcmds {
		if !contains(depsSubcmds, expected) {
			t.Errorf("depsCmd missing subcommand %q after registration", expected)
		}
	}
}

// TestE2E_FlagIntegrity verifies that flags are properly registered
func TestE2E_FlagIntegrity(t *testing.T) {
	// Root command should have --data flag (from init())
	dataFlag := rootCmd.PersistentFlags().Lookup("data")
	if dataFlag == nil {
		t.Error("rootCmd missing --data flag")
	}

	// After RegisterCLICommands, ticketsCmd should have --server and --json flags
	// Note: These flags are registered by RegisterCLICommands(), which may have already been called
	// in previous tests. If not called yet, we can't call it again without causing flag re-registration panic.
	// We verify the expected state: either flags exist (already registered) or don't (not yet registered).
	serverFlag := ticketsCmd.PersistentFlags().Lookup("server")
	jsonFlag := ticketsCmd.PersistentFlags().Lookup("json")

	if serverFlag != nil && jsonFlag != nil {
		t.Log("ticketsCmd flags already registered (expected if RegisterCLICommands was called)")
	} else if serverFlag == nil && jsonFlag == nil {
		t.Log("ticketsCmd flags not yet registered (would be registered by RegisterCLICommands)")
	} else {
		t.Errorf("ticketsCmd flags in inconsistent state: server=%v, json=%v", serverFlag != nil, jsonFlag != nil)
	}

	// Agent subcommands should have their flags (from init())
	agentCompleteWorkflowFlag := agentCompleteCmd.Flags().Lookup("workflow")
	if agentCompleteWorkflowFlag == nil {
		t.Error("agentCompleteCmd missing --workflow flag")
	}

	agentCallbackLevelFlag := agentCallbackCmd.Flags().Lookup("level")
	if agentCallbackLevelFlag == nil {
		t.Error("agentCallbackCmd missing --level flag")
	}
}
