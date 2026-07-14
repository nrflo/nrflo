package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Fail-closed coverage for the --console PreToolUse hook: every path that
// yields no human decision must still print a deny hookSpecificOutput and
// exit 0, and the timeout ladder that makes a slow approval reachable at all.

// TestAgentRecordEventCmd_ConsoleServerDown_PrintsDeny covers the other
// no-decision path: with the server not running there is nobody to ask, so a
// --console PreToolUse must still fail closed. Printing nothing would let
// claude fall back to its own interactive permission prompt — the exact
// outcome the deny exists to prevent. A plain (non-console) hook still exits
// silently; see TestAgentRecordEventCmd_NonConsoleServerDown_Silent.
func TestAgentRecordEventCmd_ConsoleServerDown_PrintsDeny(t *testing.T) {
	t.Setenv("NRFLO_SOCKET", filepath.Join(t.TempDir(), "does-not-exist.sock"))
	t.Setenv("NRF_SESSION_ID", "sess-console-server-down")
	t.Setenv("NRF_WORKFLOW_INSTANCE_ID", "")
	setConsoleFlag(t, true)

	var buf bytes.Buffer
	agentRecordEventCmd.SetOut(&buf)
	agentRecordEventCmd.SetErr(&buf)
	agentRecordEventCmd.SetIn(strings.NewReader(`{"hook_event_name":"PreToolUse","tool_name":"Bash"}`))

	if err := agentRecordEventCmd.RunE(agentRecordEventCmd, nil); err != nil {
		t.Fatalf("RunE returned error, want nil (exit 0) with the server down, got: %v", err)
	}
	want := renderConsoleDenyDecision(consoleApprovalTransportReason)
	if got := strings.TrimSpace(buf.String()); got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

// TestAgentRecordEventCmd_NonConsoleServerDown_Silent guards the autonomous
// path: hooks must not block the agent, so a plain record-event with the
// server down still prints nothing and exits 0.
func TestAgentRecordEventCmd_NonConsoleServerDown_Silent(t *testing.T) {
	t.Setenv("NRFLO_SOCKET", filepath.Join(t.TempDir(), "does-not-exist.sock"))
	t.Setenv("NRF_SESSION_ID", "sess-autonomous-server-down")
	setConsoleFlag(t, false)

	var buf bytes.Buffer
	agentRecordEventCmd.SetOut(&buf)
	agentRecordEventCmd.SetErr(&buf)
	agentRecordEventCmd.SetIn(strings.NewReader(`{"hook_event_name":"PreToolUse","tool_name":"Bash"}`))

	if err := agentRecordEventCmd.RunE(agentRecordEventCmd, nil); err != nil {
		t.Fatalf("RunE returned error, want nil (exit 0): %v", err)
	}
	if got := strings.TrimSpace(buf.String()); got != "" {
		t.Errorf("output = %q, want empty (autonomous hooks stay silent when the server is down)", got)
	}
}

// TestConsoleHookReadDeadline_AboveCLIDeadline pins the two CLI-side rungs of
// the approval timeout ladder (server 600s < CLI select 630s < socket read
// 660s ≈ the settings hook timeout). Client.Execute's DEFAULT read deadline is
// 5 minutes — below the server's 600s approval wait — so the console path must
// pass an explicit deadline that outlasts its own select; otherwise a slow
// human approval is denied by the socket read while the engine still records
// their "allow", and CLI behavior silently diverges from the decision.
func TestConsoleHookReadDeadline_AboveCLIDeadline(t *testing.T) {
	if consoleHookReadDeadline <= consoleHookDeadline {
		t.Errorf("consoleHookReadDeadline (%s) must exceed consoleHookDeadline (%s)", consoleHookReadDeadline, consoleHookDeadline)
	}
	if consoleHookDeadline <= 600*time.Second {
		t.Errorf("consoleHookDeadline (%s) must exceed the server-side approval wait (600s)", consoleHookDeadline)
	}
}
