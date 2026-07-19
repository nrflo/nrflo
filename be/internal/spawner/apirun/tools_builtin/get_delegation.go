package tools_builtin

import (
	"context"
	"encoding/json"
	"strings"
	"time"

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
		Description: "Poll an async delegation started via delegate. Returns aggregated worker findings plus per-worker status; set wait_sec to block up to that many seconds (max 240) for still-running workers.",
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
		return result, delegationStatus(result) == "failed", nil
	}
	return pollDelegation(ctx, env, args.DelegationID, args.WaitSec)
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
			return last, false, nil
		case <-deadline.C:
			return last, false, nil
		case <-ticker.C:
			heartbeatEvery++
			if env.Heartbeat != nil && heartbeatEvery%10 == 0 { // ~every 30s
				env.Heartbeat()
			}
		}
	}
}
