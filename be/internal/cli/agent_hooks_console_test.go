package cli

import (
	"bufio"
	"bytes"
	"encoding/json"
	"net"
	"strings"
	"testing"

	"be/internal/socket"
)

func mustDecodeHookResp(t *testing.T, jsonStr string) hookRecordEventResponse {
	t.Helper()
	var resp hookRecordEventResponse
	if err := json.Unmarshal([]byte(jsonStr), &resp); err != nil {
		t.Fatalf("unmarshal %q: %v", jsonStr, err)
	}
	return resp
}

// TestRenderHookDecision_PermissionDecision is the golden test for a console
// PreToolUse response: renderHookDecision must emit the exact
// hookSpecificOutput shape the installed Claude CLI (2.1.209) expects —
// `decision` is deprecated for PreToolUse, so only permissionDecision is used.
func TestRenderHookDecision_PermissionDecision(t *testing.T) {
	resp := mustDecodeHookResp(t, `{"permission_decision":{"decision":"allow","reason":"human approved"}}`)
	got := renderHookDecision(resp)
	want := `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"allow","permissionDecisionReason":"human approved"}}`
	if got != want {
		t.Errorf("renderHookDecision =\n  %s\nwant:\n  %s", got, want)
	}
}

func TestRenderHookDecision_PermissionDecisionDeny(t *testing.T) {
	resp := mustDecodeHookResp(t, `{"permission_decision":{"decision":"deny","reason":"human denied"}}`)
	got := renderHookDecision(resp)
	want := `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"human denied"}}`
	if got != want {
		t.Errorf("renderHookDecision =\n  %s\nwant:\n  %s", got, want)
	}
}

// TestRenderHookDecision_StopDecisionBlock pins the Stop/PostToolUse/
// UserPromptSubmit shape (still `decision:"block"`) — left untouched by the
// console PreToolUse change per handler_stop.go/agent_hooks.go:109-115.
func TestRenderHookDecision_StopDecisionBlock(t *testing.T) {
	resp := mustDecodeHookResp(t, `{"stop_decision":{"block":true,"reason":"keep going"}}`)
	got := renderHookDecision(resp)
	want := `{"decision":"block","reason":"keep going"}`
	if got != want {
		t.Errorf("renderHookDecision =\n  %s\nwant:\n  %s", got, want)
	}
}

func TestRenderHookDecision_StopDecisionNotBlocked_ReturnsEmpty(t *testing.T) {
	resp := mustDecodeHookResp(t, `{"stop_decision":{"block":false,"reason":""}}`)
	if got := renderHookDecision(resp); got != "" {
		t.Errorf("renderHookDecision = %q, want empty for a non-blocking stop_decision", got)
	}
}

func TestRenderHookDecision_NoFields_ReturnsEmpty(t *testing.T) {
	resp := mustDecodeHookResp(t, `{"status":"recorded"}`)
	if got := renderHookDecision(resp); got != "" {
		t.Errorf("renderHookDecision = %q, want empty when neither decision field is present", got)
	}
}

func TestRenderHookDecision_PermissionDecisionTakesPrecedenceOverStopDecision(t *testing.T) {
	resp := mustDecodeHookResp(t, `{"permission_decision":{"decision":"deny","reason":"no"},"stop_decision":{"block":true,"reason":"ignored"}}`)
	got := renderHookDecision(resp)
	if !strings.Contains(got, "hookSpecificOutput") || strings.Contains(got, "ignored") {
		t.Errorf("permission_decision must take precedence when both are present, got %s", got)
	}
}

// TestRenderConsoleDenyDecision_Shape is the golden test for the fail-closed
// deny JSON printed on a --console PreToolUse deadline or transport error.
func TestRenderConsoleDenyDecision_Shape(t *testing.T) {
	got := renderConsoleDenyDecision("nrflo: approval timed out")
	want := `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"nrflo: approval timed out"}}`
	if got != want {
		t.Errorf("renderConsoleDenyDecision =\n  %s\nwant:\n  %s", got, want)
	}
}

// TestAgentRecordEventCmd_ConsoleFlagRegistered asserts --console is
// registered on the record-event command, defaulting to false (plain
// autonomous hook behavior unless explicitly opted in).
func TestAgentRecordEventCmd_ConsoleFlagRegistered(t *testing.T) {
	flag := agentRecordEventCmd.Flags().Lookup("console")
	if flag == nil {
		t.Fatal("agentRecordEventCmd missing --console flag")
	}
	if flag.DefValue != "false" {
		t.Errorf("--console default = %q, want false", flag.DefValue)
	}
}

// startFakeRecordEventServer listens on a temp unix socket (path from
// shortTempSocket, serve_setup_test.go's helper) and, for every connection,
// reads one JSON-RPC request line and writes back respond(req) as the
// response line. Lets a test drive agentRecordEventCmd.RunE through the real
// Client.ExecuteAndUnmarshal transport without a real nrflo server.
func startFakeRecordEventServer(t *testing.T, respond func(req socket.Request) socket.Response) string {
	t.Helper()
	path := shortTempSocket(t)
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatalf("listen unix %s: %v", path, err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				line, err := bufio.NewReader(c).ReadBytes('\n')
				if err != nil {
					return
				}
				var req socket.Request
				if err := json.Unmarshal(line, &req); err != nil {
					return
				}
				out, _ := json.Marshal(respond(req))
				c.Write(append(out, '\n'))
			}(conn)
		}
	}()
	return path
}

// setConsoleFlag sets agentRecordEventConsole for the duration of the test
// and restores the prior value on cleanup (it is a shared package var mutated
// by the real --console cobra flag binding).
func setConsoleFlag(t *testing.T, v bool) {
	t.Helper()
	prev := agentRecordEventConsole
	agentRecordEventConsole = v
	t.Cleanup(func() { agentRecordEventConsole = prev })
}

// TestAgentRecordEventCmd_ConsoleTransportError_PrintsDenyAndExitsZero is the
// end-to-end acceptance case: a --console PreToolUse call whose transport
// fails (server returns a JSON-RPC error) must print the fail-closed deny
// JSON and return nil (exit 0), never surfacing the raw transport error to
// Claude's hook stdout protocol.
func TestAgentRecordEventCmd_ConsoleTransportError_PrintsDenyAndExitsZero(t *testing.T) {
	sockPath := startFakeRecordEventServer(t, func(req socket.Request) socket.Response {
		return socket.Response{ID: req.ID, Error: &socket.ErrorInfo{Code: socket.ErrCodeInternal, Message: "boom"}}
	})
	t.Setenv("NRFLO_SOCKET", sockPath)
	t.Setenv("NRF_SESSION_ID", "sess-console-transport-err")
	t.Setenv("NRF_WORKFLOW_INSTANCE_ID", "")
	setConsoleFlag(t, true)

	var buf bytes.Buffer
	agentRecordEventCmd.SetOut(&buf)
	agentRecordEventCmd.SetErr(&buf)
	agentRecordEventCmd.SetIn(strings.NewReader(`{"hook_event_name":"PreToolUse","tool_name":"Bash"}`))

	if err := agentRecordEventCmd.RunE(agentRecordEventCmd, nil); err != nil {
		t.Fatalf("RunE returned error, want nil (exit 0) on a --console transport error, got: %v", err)
	}
	want := renderConsoleDenyDecision(consoleApprovalTransportReason)
	if got := strings.TrimSpace(buf.String()); got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

// TestAgentRecordEventCmd_NonConsole_TransportError_PropagatesError is the
// regression guard: without --console, a PreToolUse transport error must
// still propagate as a real error (today's autonomous behavior), not get
// silently swallowed into a deny decision.
func TestAgentRecordEventCmd_NonConsole_TransportError_PropagatesError(t *testing.T) {
	sockPath := startFakeRecordEventServer(t, func(req socket.Request) socket.Response {
		return socket.Response{ID: req.ID, Error: &socket.ErrorInfo{Code: socket.ErrCodeInternal, Message: "boom"}}
	})
	t.Setenv("NRFLO_SOCKET", sockPath)
	t.Setenv("NRF_SESSION_ID", "sess-plain-transport-err")
	t.Setenv("NRF_WORKFLOW_INSTANCE_ID", "")
	setConsoleFlag(t, false)

	var buf bytes.Buffer
	agentRecordEventCmd.SetOut(&buf)
	agentRecordEventCmd.SetErr(&buf)
	agentRecordEventCmd.SetIn(strings.NewReader(`{"hook_event_name":"PreToolUse","tool_name":"Bash"}`))

	if err := agentRecordEventCmd.RunE(agentRecordEventCmd, nil); err == nil {
		t.Fatal("expected RunE to return an error for a non-console PreToolUse transport error")
	}
	if buf.Len() != 0 {
		t.Errorf("non-console error path should print nothing, got %q", buf.String())
	}
}

// TestAgentRecordEventCmd_ServerNotRunning_SilentExit pins the "hooks must
// not block the agent" contract when the server isn't up at all — no socket
// listener at NRFLO_SOCKET, so IsServerRunning() fails fast.
func TestAgentRecordEventCmd_ServerNotRunning_SilentExit(t *testing.T) {
	t.Setenv("NRFLO_SOCKET", shortTempSocket(t)) // path exists but nothing listens
	t.Setenv("NRF_SESSION_ID", "sess-no-server")

	var buf bytes.Buffer
	agentRecordEventCmd.SetOut(&buf)
	agentRecordEventCmd.SetErr(&buf)
	agentRecordEventCmd.SetIn(strings.NewReader(`{"hook_event_name":"PreToolUse"}`))

	if err := agentRecordEventCmd.RunE(agentRecordEventCmd, nil); err != nil {
		t.Errorf("RunE with no server running should return nil, got: %v", err)
	}
	if buf.Len() != 0 {
		t.Errorf("no-server path should print nothing, got %q", buf.String())
	}
}

func TestConsoleTimeoutLadder(t *testing.T) {
	const serverApprovalWaitSec = 600
	const settingsPreToolUseTimeoutSec = 660
	cliDeadlineSec := int(consoleHookDeadline.Seconds())

	if cliDeadlineSec <= serverApprovalWaitSec {
		t.Errorf("consoleHookDeadline=%ds must exceed the server-side approval wait (%ds)", cliDeadlineSec, serverApprovalWaitSec)
	}
	if cliDeadlineSec >= settingsPreToolUseTimeoutSec {
		t.Errorf("consoleHookDeadline=%ds must stay under the settings PreToolUse timeout (%ds)", cliDeadlineSec, settingsPreToolUseTimeoutSec)
	}
}
