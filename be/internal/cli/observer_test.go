package cli

// Tests run sequentially (no t.Parallel) — cobra Hidden flags and env vars
// (NRF_OBSERVER, NRF_OBSERVER_SCOPE) are process-global state.

import (
	"testing"

	"github.com/spf13/cobra"
)

// newObserverRoot adds observerCmd under a fresh cobra root (Use="nrflo") so
// that cmd.Root().Name() returns "nrflo" from within the observer tree.
// Each call updates observerCmd.parent; sequential tests call this before use.
func newObserverRoot() *cobra.Command {
	root := &cobra.Command{Use: "nrflo"}
	root.AddCommand(observerCmd)
	return root
}

// restoreGroupHidden saves the current Hidden state of the four observer
// commands and registers a cleanup to restore them after the test.
func restoreGroupHidden(t *testing.T) {
	t.Helper()
	so := observerCmd.Hidden
	sw := workflowGroupCmd.Hidden
	sp := projectGroupCmd.Hidden
	sg := globalGroupCmd.Hidden
	t.Cleanup(func() {
		observerCmd.Hidden = so
		workflowGroupCmd.Hidden = sw
		projectGroupCmd.Hidden = sp
		globalGroupCmd.Hidden = sg
	})
}

// TestObserverCmd_EnvUnset verifies that PersistentPreRunE returns the
// cobra-style unknown-command error and leaves observerCmd.Hidden=true when
// NRF_OBSERVER is not set to "1".
func TestObserverCmd_EnvUnset(t *testing.T) {
	newObserverRoot()
	restoreGroupHidden(t)
	t.Setenv("NRF_OBSERVER", "")
	observerCmd.Hidden = true

	err := observerCmd.PersistentPreRunE(observerCmd, []string{})
	if err == nil {
		t.Fatal("PersistentPreRunE: expected error when NRF_OBSERVER unset, got nil")
	}
	want := unknownCmdErr("observer", "nrflo").Error()
	if err.Error() != want {
		t.Errorf("error=%q want %q", err.Error(), want)
	}
	if !observerCmd.Hidden {
		t.Error("observerCmd.Hidden should remain true when NRF_OBSERVER unset")
	}
}

// TestObserverCmd_HelpHidden verifies that observer does not appear as an
// available command (filtered via IsAvailableCommand) when NRF_OBSERVER unset.
func TestObserverCmd_HelpHidden(t *testing.T) {
	testRoot := newObserverRoot()
	restoreGroupHidden(t)
	t.Setenv("NRF_OBSERVER", "")
	observerCmd.Hidden = true

	for _, cmd := range testRoot.Commands() {
		if cmd.IsAvailableCommand() && cmd.Name() == "observer" {
			t.Error("observer should NOT appear as available command when NRF_OBSERVER unset")
		}
	}
}

// TestObserverCmd_ScopeMatrix verifies that for each valid scope
// (workflow/project/global):
//   - observerCmd.PersistentPreRunE returns nil and unhides observerCmd
//   - Exactly the matching group is unhidden; the other two remain hidden
//   - Each group's PersistentPreRunE returns nil for in-scope calls and
//     the correct unknown-command error for out-of-scope calls
func TestObserverCmd_ScopeMatrix(t *testing.T) {
	newObserverRoot()
	restoreGroupHidden(t)

	type groupEntry struct {
		cmd   *cobra.Command
		scope string
	}
	groups := []groupEntry{
		{workflowGroupCmd, "workflow"},
		{projectGroupCmd, "project"},
		{globalGroupCmd, "global"},
	}

	for _, activeScope := range []string{"workflow", "project", "global"} {
		activeScope := activeScope
		t.Run(activeScope, func(t *testing.T) {
			t.Setenv("NRF_OBSERVER", "1")
			t.Setenv("NRF_OBSERVER_SCOPE", activeScope)

			if err := observerCmd.PersistentPreRunE(observerCmd, []string{}); err != nil {
				t.Fatalf("observerCmd PersistentPreRunE(scope=%q): %v", activeScope, err)
			}
			if observerCmd.Hidden {
				t.Errorf("scope=%q: observerCmd.Hidden want false, got true", activeScope)
			}

			for _, g := range groups {
				wantHidden := g.scope != activeScope
				if g.cmd.Hidden != wantHidden {
					t.Errorf("scope=%q: %sGroupCmd.Hidden=%v want %v",
						activeScope, g.scope, g.cmd.Hidden, wantHidden)
				}
				err := g.cmd.PersistentPreRunE(g.cmd, []string{})
				if g.scope == activeScope {
					if err != nil {
						t.Errorf("scope=%q: in-scope %s PersistentPreRunE: unexpected error: %v",
							activeScope, g.scope, err)
					}
				} else {
					wantErr := unknownCmdErr(g.scope, "nrflo").Error()
					if err == nil {
						t.Errorf("scope=%q: out-of-scope %s PersistentPreRunE: want error %q, got nil",
							activeScope, g.scope, wantErr)
					} else if err.Error() != wantErr {
						t.Errorf("scope=%q: out-of-scope %s error=%q want %q",
							activeScope, g.scope, err.Error(), wantErr)
					}
				}
			}
		})
	}
}

// TestObserverCmd_OutOfScopeLeaf verifies that calling projectGroupCmd's
// PersistentPreRunE while the scope is set to "workflow" returns the
// expected unknown-command error.
func TestObserverCmd_OutOfScopeLeaf(t *testing.T) {
	newObserverRoot()
	t.Setenv("NRF_OBSERVER", "1")
	t.Setenv("NRF_OBSERVER_SCOPE", "workflow")

	err := projectGroupCmd.PersistentPreRunE(projectGroupCmd, []string{})
	if err == nil {
		t.Fatal("expected error for out-of-scope projectGroupCmd call, got nil")
	}
	want := unknownCmdErr("project", "nrflo").Error()
	if err.Error() != want {
		t.Errorf("error=%q want %q", err.Error(), want)
	}
}

// TestObserverCmd_HiddenFlipIdempotent verifies that PersistentPreRunE can be
// called multiple times with different scopes and the Hidden flags update
// correctly each time.
func TestObserverCmd_HiddenFlipIdempotent(t *testing.T) {
	newObserverRoot()
	restoreGroupHidden(t)
	t.Setenv("NRF_OBSERVER", "1")

	// First call: scope=workflow
	t.Setenv("NRF_OBSERVER_SCOPE", "workflow")
	if err := observerCmd.PersistentPreRunE(observerCmd, []string{}); err != nil {
		t.Fatalf("first call (workflow): %v", err)
	}
	if workflowGroupCmd.Hidden {
		t.Error("after first call: workflowGroupCmd.Hidden want false, got true")
	}
	if !projectGroupCmd.Hidden {
		t.Error("after first call: projectGroupCmd.Hidden want true, got false")
	}
	if !globalGroupCmd.Hidden {
		t.Error("after first call: globalGroupCmd.Hidden want true, got false")
	}

	// Second call: scope=project — flags should update correctly
	t.Setenv("NRF_OBSERVER_SCOPE", "project")
	if err := observerCmd.PersistentPreRunE(observerCmd, []string{}); err != nil {
		t.Fatalf("second call (project): %v", err)
	}
	if !workflowGroupCmd.Hidden {
		t.Error("after second call: workflowGroupCmd.Hidden want true, got false")
	}
	if projectGroupCmd.Hidden {
		t.Error("after second call: projectGroupCmd.Hidden want false, got true")
	}
	if !globalGroupCmd.Hidden {
		t.Error("after second call: globalGroupCmd.Hidden want true, got false")
	}
}

// TestObserverCmd_Subcommands verifies the structural composition of the
// observer cobra tree registered via init().
func TestObserverCmd_Subcommands(t *testing.T) {
	groups := getCommandNames(observerCmd)
	for _, want := range []string{"workflow", "project", "global"} {
		if !contains(groups, want) {
			t.Errorf("observerCmd missing group %q; got %v", want, groups)
		}
	}
	if len(groups) != 3 {
		t.Errorf("observerCmd has %d groups want 3; %v", len(groups), groups)
	}

	for _, name := range []string{"show", "runs", "findings", "logs", "trigger", "retry-failed", "def"} {
		if !contains(getCommandNames(workflowGroupCmd), name) {
			t.Errorf("workflowGroupCmd missing subcommand %q", name)
		}
	}
	for _, name := range []string{"workflows", "runs", "findings", "env", "workflow"} {
		if !contains(getCommandNames(projectGroupCmd), name) {
			t.Errorf("projectGroupCmd missing subcommand %q", name)
		}
	}
	for _, name := range []string{"projects", "recent-sessions", "health", "project"} {
		if !contains(getCommandNames(globalGroupCmd), name) {
			t.Errorf("globalGroupCmd missing subcommand %q", name)
		}
	}
}
