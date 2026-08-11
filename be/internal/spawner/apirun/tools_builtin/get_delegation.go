package tools_builtin

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"be/internal/model"
	"be/internal/spawner/apirun"
	"be/internal/spawner/apirun/provider"
)

// delegateMaxWaitSec caps the optional bounded wait on both delegate tools,
// mirroring subworkflowMaxWaitSec.
const delegateMaxWaitSec = 240

// delegatePollInterval is the poll cadence during a bounded wait. A var so
// tests can shrink it.
var delegatePollInterval = 3 * time.Second

type getDelegationHandler struct{}

func (getDelegationHandler) Spec() provider.ToolSpec {
	return provider.ToolSpec{
		Name:        "get_delegation",
		Description: "Collect an async delegation started via delegate. ALWAYS pass wait_sec to block up to that many seconds (max 240) for still-running workers — one blocking call, never a bare-poll loop. Returns {delegation_id, status, results?}: status running while any worker still runs (results list per-worker progress), then completed — or failed when at least one worker failed — with results[{session_id, status, reason?, findings?}] where findings is each worker's structured output. READ-ONCE: the terminal response is consumed as it is returned — worker findings and the delegation record are deleted — so store the results; polling the same delegation_id again returns an unknown-delegation error, not a repeat of the data. Under a CLI console engine a wait over ~120s may return as a background-task notification carrying a non-terminal status — that notification does not consume the delegation: call get_delegation once more after it, and never treat the backgrounding as an error.",
		InputSchema: json.RawMessage(`{
"type":"object",
"properties":{
"delegation_id":{"type":"string"},
"wait_sec":{"type":"integer","description":"Block up to this many seconds (max 240) for the remaining workers to finish"}
},
"required":["delegation_id"],
"additionalProperties":false
}`),
	}
}

func (getDelegationHandler) Invoke(ctx context.Context, env apirun.ToolEnv, input json.RawMessage) (string, bool, error) {
	var args struct {
		DelegationID string `json:"delegation_id"`
		WaitSec      int    `json:"wait_sec"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return invalidArgs(err)
	}
	if env.Delegator == nil {
		return missingService("delegator")
	}
	if strings.TrimSpace(args.DelegationID) == "" {
		return "delegation_id is required", true, nil
	}
	if args.WaitSec <= 0 {
		result, err := env.Delegator.GetDelegation(ctx, env.SessionID, args.DelegationID)
		if err != nil {
			return err.Error(), true, nil
		}
		return appendPollHint(env, result), delegationStatus(result) == "failed", nil
	}
	return pollDelegation(ctx, env, args.DelegationID, args.WaitSec)
}

// appendPollHint stamps a "hint" field onto a still-running result returned
// from a non-blocking call. For a console chat the hint points at the
// notification contract (the ChatNotifier delivers completion as a turn);
// everywhere else it steers the model to a single bounded wait — an async
// return with no hint reliably produces a get_delegation call per model turn
// until the workers finish.
func appendPollHint(env apirun.ToolEnv, raw string) string {
	var v map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &v); err != nil || v["status"] != "running" {
		return raw
	}
	if env.SessionKind == model.AgentSessionKindConsoleChat {
		v["hint"] = "running async — end your turn now; this chat is notified when the delegation completes, then collect it with ONE get_delegation call. Never poll and never block on waits."
	} else {
		v["hint"] = "still running — collect with one get_delegation call passing wait_sec (max 240), do not re-poll without it. Under a CLI console engine a wait over ~120s may return as a background-task notification carrying a non-terminal status — that notification does not consume the delegation: call get_delegation once more after it, and never treat the backgrounding as an error."
	}
	b, err := json.Marshal(v)
	if err != nil {
		return raw
	}
	return string(b)
}

// delegationStatus extracts the top-level "status" field from a delegate/
// get_delegation JSON result ("" when unparsable).
func delegationStatus(raw string) string {
	var v struct {
		Status string `json:"status"`
	}
	json.Unmarshal([]byte(raw), &v) //nolint:errcheck
	return v.Status
}

// pollDelegation waits up to waitSec (capped) for a delegation to leave the
// "running" state, heartbeating the caller so stall detection stays quiet,
// and returns the current result either way (never an error on timeout — the
// caller can keep polling with get_delegation). Mirrors pollSubworkflow.
func pollDelegation(ctx context.Context, env apirun.ToolEnv, delegationID string, waitSec int) (string, bool, error) {
	if waitSec > delegateMaxWaitSec {
		waitSec = delegateMaxWaitSec
	}
	deadline := time.NewTimer(time.Duration(waitSec) * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(delegatePollInterval)
	defer ticker.Stop()
	heartbeatEvery := 0
	var last string

	for {
		result, err := env.Delegator.GetDelegation(ctx, env.SessionID, delegationID)
		if err != nil {
			return err.Error(), true, nil
		}
		last = result
		if status := delegationStatus(result); status != "running" {
			return result, status == "failed", nil
		}
		select {
		case <-ctx.Done():
			return appendPollHint(env, last), false, nil
		case <-deadline.C:
			return appendPollHint(env, last), false, nil
		case <-ticker.C:
			heartbeatEvery++
			if env.Heartbeat != nil && heartbeatEvery%10 == 0 { // ~every 30s
				env.Heartbeat()
			}
		}
	}
}
