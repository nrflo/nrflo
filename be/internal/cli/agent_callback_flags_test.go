package cli

import (
	"strings"
	"testing"
)

// TestAgentCallbackCmd_MutexValidation verifies that the command rejects conflicting flag combinations.
// Tests run sequentially (no t.Parallel) because cobra's Changed flag state is package-level.
func TestAgentCallbackCmd_MutexValidation(t *testing.T) {
	// "none set" must run before any level flag is set so Changed("level") is false.
	t.Run("none set", func(t *testing.T) {
		agentCallbackTargetAgent = ""
		agentCallbackChain = nil
		err := agentCallbackCmd.RunE(agentCallbackCmd, nil)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "exactly one of") {
			t.Errorf("expected mutex error, got: %v", err)
		}
	})

	// Set level flag once; cobra marks it Changed for all remaining sub-tests.
	if err := agentCallbackCmd.Flags().Set("level", "2"); err != nil {
		t.Fatalf("failed to set level flag: %v", err)
	}
	agentCallbackLevel = 2

	for _, tc := range []struct {
		name        string
		targetAgent string
		chain       []string
	}{
		{"level and agent", "foo", nil},
		{"level and chain", "", []string{"a", "b"}},
		{"agent and chain (level still changed)", "foo", []string{"a"}},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			agentCallbackTargetAgent = tc.targetAgent
			agentCallbackChain = tc.chain
			err := agentCallbackCmd.RunE(agentCallbackCmd, nil)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), "exactly one of") {
				t.Errorf("expected mutex error, got: %v", err)
			}
		})
	}

	// Cleanup after test.
	agentCallbackLevel = 0
	agentCallbackTargetAgent = ""
	agentCallbackChain = nil
}

// TestAgentCallbackCmd_FlagRegistration verifies that agentCallbackCmd has all expected flags.
func TestAgentCallbackCmd_FlagRegistration(t *testing.T) {
	t.Parallel()

	// --agent flag: string
	agentFlag := agentCallbackCmd.Flags().Lookup("agent")
	if agentFlag == nil {
		t.Fatal("missing --agent flag on agentCallbackCmd")
	}
	if agentFlag.Value.Type() != "string" {
		t.Errorf("--agent flag type = %q, want string", agentFlag.Value.Type())
	}

	// --chain flag: stringSlice
	chainFlag := agentCallbackCmd.Flags().Lookup("chain")
	if chainFlag == nil {
		t.Fatal("missing --chain flag on agentCallbackCmd")
	}
	if chainFlag.Value.Type() != "stringSlice" {
		t.Errorf("--chain flag type = %q, want stringSlice", chainFlag.Value.Type())
	}

	// --level flag: int
	levelFlag := agentCallbackCmd.Flags().Lookup("level")
	if levelFlag == nil {
		t.Fatal("missing --level flag on agentCallbackCmd")
	}
	if levelFlag.Value.Type() != "int" {
		t.Errorf("--level flag type = %q, want int", levelFlag.Value.Type())
	}
}

// TestAgentCallbackCmd_RegisteredUnderAgentCmd verifies that agentCallbackCmd is a
// subcommand of agentCmd.
func TestAgentCallbackCmd_RegisteredUnderAgentCmd(t *testing.T) {
	t.Parallel()

	subNames := getCommandNames(agentCmd)
	if !contains(subNames, "callback") {
		t.Errorf("agentCmd subcommands = %v, want 'callback' present", subNames)
	}
}
