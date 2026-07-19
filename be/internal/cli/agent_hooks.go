package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"
)

// These commands are agent infrastructure invoked by the spawner / Claude hooks
// (not by the agent itself). They are registered on nrflo_server's agentInfraCmd
// in agent_infra.go and connect back to the running server over the Unix socket.

// context-update flag.
var agentContextUpdatePctUsed float64

var agentContextUpdateCmd = &cobra.Command{
	Use:   "context-update <session-id>",
	Short: "Update context usage for an agent session (used by hooks)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// No RequireProject — session_id is globally unique
		if err := CheckServer(); err != nil {
			// Server not running — exit silently
			return nil
		}

		sessionID := args[0]
		contextLeft := int(100 - agentContextUpdatePctUsed)
		if contextLeft < 0 {
			contextLeft = 0
		}
		if contextLeft > 100 {
			contextLeft = 100
		}

		c := GetClient()
		reqParams := map[string]interface{}{
			"session_id":   sessionID,
			"context_left": contextLeft,
		}

		return c.ExecuteAndUnmarshal("agent.context_update", reqParams, nil)
	},
}

// agentRecordEventConsole is set by --console: the hook command a console
// (human-attended) session's --settings JSON registers
// (BuildConsoleSettingsJSON). It changes two things for a PreToolUse event:
// the CLI deadline extends to consoleHookDeadline (the server blocks that
// call on a human approval decision), and the response's permission_decision
// is rendered as PreToolUse hookSpecificOutput JSON.
var agentRecordEventConsole bool

// recordEventDeadline is the hard deadline for a plain (non-console) hook
// call — hooks must not block the agent.
const recordEventDeadline = 2 * time.Second

// consoleHookDeadline is the CLI-side deadline for a --console PreToolUse
// call: the middle rung of the timeout ladder (server-side approval wait
// 600s < this 630s < the settings PreToolUse hook `timeout` 660s) — see
// spawner/REFERENCE.md.
const consoleHookDeadline = 630 * time.Second

// consoleHookReadDeadline is the socket read deadline used for a --console
// PreToolUse call. Client.Execute's default is 5 minutes — BELOW the server's
// 600s approval wait — so the call must use the explicit-deadline variant or
// the socket read would time out first and deny an approval the human is
// still answering. Kept above consoleHookDeadline so the select below, not
// the socket, owns the CLI-side timeout.
const consoleHookReadDeadline = consoleHookDeadline + 30*time.Second

// Fail-closed deny reasons for a --console PreToolUse call that never got a
// decision from the server: denying is safer than letting claude fall back to
// its own interactive permission prompt, which nobody can answer in a
// server-driven PTY.
const (
	consoleApprovalTimeoutReason   = "nrflo: approval timed out"
	consoleApprovalTransportReason = "nrflo: could not reach the nrflo server for an approval decision"
)

// hookRecordEventResponse is the subset of agent.record_event's JSON-RPC
// result the CLI renders back onto Claude's hook stdout protocol.
type hookRecordEventResponse struct {
	StopDecision *struct {
		Block  bool   `json:"block"`
		Reason string `json:"reason"`
	} `json:"stop_decision"`
	PermissionDecision *struct {
		Decision string `json:"decision"`
		Reason   string `json:"reason"`
	} `json:"permission_decision"`
	AdditionalContext string `json:"additional_context,omitempty"`
}

// renderHookDecision renders a successful agent.record_event response into
// Claude's hook control-output JSON, or "" when there is nothing to print.
// Pure — no socket, no CLI execution — so it is unit-testable directly.
func renderHookDecision(resp hookRecordEventResponse) string {
	// PreToolUse (console only): `decision` is deprecated for this hook per
	// the installed CLI's own docs — permissionDecision is the correct shape.
	if resp.PermissionDecision != nil {
		out, _ := json.Marshal(map[string]interface{}{
			"hookSpecificOutput": map[string]interface{}{
				"hookEventName":            "PreToolUse",
				"permissionDecision":       resp.PermissionDecision.Decision,
				"permissionDecisionReason": resp.PermissionDecision.Reason,
			},
		})
		return string(out)
	}
	// Stop-hook control output: blocks the turn-end and feeds `reason` back
	// to the model so it keeps going until it calls a completion tool.
	if resp.StopDecision != nil && resp.StopDecision.Block {
		out, _ := json.Marshal(map[string]interface{}{
			"decision": "block",
			"reason":   resp.StopDecision.Reason,
		})
		return string(out)
	}
	// UserPromptSubmit (console only): additionalContext surfaces
	// ContextInjector output ahead of the model's next turn.
	if resp.AdditionalContext != "" {
		out, _ := json.Marshal(map[string]interface{}{
			"hookSpecificOutput": map[string]interface{}{
				"hookEventName":     "UserPromptSubmit",
				"additionalContext": resp.AdditionalContext,
			},
		})
		return string(out)
	}
	return ""
}

// renderConsoleDenyDecision renders the fail-closed PreToolUse deny JSON
// printed on a --console record-event deadline or transport error.
func renderConsoleDenyDecision(reason string) string {
	out, _ := json.Marshal(map[string]interface{}{
		"hookSpecificOutput": map[string]interface{}{
			"hookEventName":            "PreToolUse",
			"permissionDecision":       "deny",
			"permissionDecisionReason": reason,
		},
	})
	return string(out)
}

var agentRecordEventCmd = &cobra.Command{
	Use:   "record-event",
	Short: "Forward a Claude hook event to the server (used by --settings hooks)",
	Long: `Read a Claude hook JSON payload from stdin and forward it to the server
via the Unix socket. Used automatically by Claude --settings PreToolUse/PostToolUse
hooks. Exits 0 on success, 1 on error. Silently exits 0 when the server is not running
(hooks must not block the agent).

Context is read from environment variables set by the spawner:
  NRF_SESSION_ID          — current agent session ID (required)
  NRF_WORKFLOW_INSTANCE_ID — workflow instance ID`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		// stdin is read (and hook_event_name resolved) BEFORE the
		// server-running / session checks: a --console PreToolUse call must
		// fail closed with a deny on every no-decision path, and whether this
		// IS such a call is only knowable from the payload.
		raw, err := io.ReadAll(cmd.InOrStdin())
		if err != nil {
			return fmt.Errorf("failed to read stdin: %w", err)
		}

		// Validate that hook_event_name is present
		var probe map[string]interface{}
		if err := json.Unmarshal(raw, &probe); err != nil {
			return fmt.Errorf("invalid hook JSON: %w", err)
		}
		hookEventName, _ := probe["hook_event_name"].(string)
		if hookEventName == "" {
			return fmt.Errorf("hook JSON missing hook_event_name field")
		}

		consolePreToolUse := agentRecordEventConsole && hookEventName == "PreToolUse"
		denyClosed := func(reason string) error {
			if consolePreToolUse {
				fmt.Fprintln(cmd.OutOrStdout(), renderConsoleDenyDecision(reason))
			}
			return nil
		}

		if !GetClient().IsServerRunning() {
			// Server not running — a plain hook exits silently (hooks must not
			// block the agent); a console PreToolUse denies.
			return denyClosed(consoleApprovalTransportReason)
		}
		if GetSessionID() == "" {
			return denyClosed(consoleApprovalTransportReason) // hook fired outside of spawner context
		}

		reqParams := map[string]interface{}{
			"event": json.RawMessage(raw),
		}
		addSpawnerIDs(reqParams)

		deadline := recordEventDeadline
		if consolePreToolUse {
			deadline = consoleHookDeadline
		}

		var resp hookRecordEventResponse
		type result struct{ err error }
		ch := make(chan result, 1)
		go func() {
			if consolePreToolUse {
				ch <- result{err: GetClient().ExecuteAndUnmarshalWithReadDeadline("agent.record_event", reqParams, &resp, consoleHookReadDeadline)}
				return
			}
			ch <- result{err: GetClient().ExecuteAndUnmarshal("agent.record_event", reqParams, &resp)}
		}()
		select {
		case r := <-ch:
			if r.err != nil {
				if consolePreToolUse {
					return denyClosed(consoleApprovalTransportReason)
				}
				return r.err
			}
			if out := renderHookDecision(resp); out != "" {
				fmt.Fprintln(cmd.OutOrStdout(), out)
			}
			return nil
		case <-time.After(deadline):
			if consolePreToolUse {
				return denyClosed(consoleApprovalTimeoutReason)
			}
			return fmt.Errorf("record-event: server did not respond within %s", deadline)
		}
	},
}
