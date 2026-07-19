package tools_builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"be/internal/service"
	"be/internal/spawner/apirun"
	"be/internal/spawner/apirun/provider"
)

// delegateMaxContextBytes caps the inline context payload; larger context
// belongs in an artifact instead.
const delegateMaxContextBytes = 4096

type delegateHandler struct{}

func (delegateHandler) Spec() provider.ToolSpec {
	return provider.ToolSpec{
		Name:        "delegate",
		Description: "Delegate work downward to a cheaper tier worker (or a fanout of them). tier=\"extractor\" answers one focused question with no further delegation; tier=\"executor\" owns a slice of work end to end and may itself delegate one level further. Returns the worker's structured findings, never a transcript. Set wait_sec to block inline (max 240s); 0 starts async and returns a delegation_id to poll with get_delegation.",
		InputSchema: json.RawMessage(`{
"type":"object",
"properties":{
"tier":{"type":"string","enum":["extractor","executor"],"description":"extractor = single-question one-shot; executor = owns a slice, may delegate one level further"},
"brief":{"type":"string","description":"What the worker should do. Templated once per fanout item when fanout is set."},
"context":{"type":"string","description":"Inline context, capped at 4KB — use an artifact for anything larger"},
"artifacts":{"type":"array","items":{"type":"string"},"description":"Names of artifacts already materialized for this run that the worker should reference"},
"wait_sec":{"type":"integer","description":"Block up to this many seconds (max 240) for the result; 0 (default) starts async"},
"fanout":{"type":"array","items":{"type":"string"},"description":"Spawn one worker per item, brief templated identically for each; omit for a single worker"}
},
"required":["tier","brief"],
"additionalProperties":false
}`),
	}
}

func (delegateHandler) Invoke(ctx context.Context, env apirun.ToolEnv, input json.RawMessage) (string, bool, error) {
	var args struct {
		Tier      string   `json:"tier"`
		Brief     string   `json:"brief"`
		Context   string   `json:"context"`
		Artifacts []string `json:"artifacts"`
		WaitSec   int      `json:"wait_sec"`
		Fanout    []string `json:"fanout"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return invalidArgs(err)
	}
	if env.Delegator == nil {
		return missingService("delegator")
	}
	if args.Tier != "extractor" && args.Tier != "executor" {
		return `tier must be "extractor" or "executor"`, true, nil
	}
	if strings.TrimSpace(args.Brief) == "" {
		return "brief is required", true, nil
	}
	if len(args.Context) > delegateMaxContextBytes {
		return fmt.Sprintf("context exceeds %d bytes; materialize an artifact instead", delegateMaxContextBytes), true, nil
	}
	if maxFanout := service.DelegateMaxFanout(env.Pool, env.ProjectID); len(args.Fanout) > maxFanout {
		return fmt.Sprintf("fanout of %d exceeds delegate_max_fanout (%d)", len(args.Fanout), maxFanout), true, nil
	}

	result, err := env.Delegator.Delegate(ctx, env.SessionID, apirun.DelegateRequest{
		Tier:      args.Tier,
		Brief:     args.Brief,
		Context:   args.Context,
		Artifacts: args.Artifacts,
		Fanout:    args.Fanout,
	})
	if err != nil {
		return err.Error(), true, nil
	}
	if args.WaitSec <= 0 {
		return result, false, nil
	}
	delegationID := delegationID(result)
	if delegationID == "" {
		return result, false, nil
	}
	return pollDelegation(ctx, env, delegationID, args.WaitSec)
}

// delegationID extracts the top-level "delegation_id" field from a delegate
// JSON result ("" when unparsable).
func delegationID(raw string) string {
	var v struct {
		DelegationID string `json:"delegation_id"`
	}
	json.Unmarshal([]byte(raw), &v) //nolint:errcheck
	return v.DelegationID
}
