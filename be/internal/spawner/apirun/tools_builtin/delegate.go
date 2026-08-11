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

// delegateDefaultInlineWaitSec is the bounded inline wait applied when an
// extractor or verifier call omits wait_sec: both are by definition quick
// one-shots, and async-by-default teaches callers to bare-poll get_delegation
// once per model turn. Explicit wait_sec:0 still starts async.
const delegateDefaultInlineWaitSec = 120

type delegateHandler struct{}

func (delegateHandler) Spec() provider.ToolSpec {
	return provider.ToolSpec{
		Name:        "delegate",
		Description: "Delegate work downward to a cheaper tier worker (or a fanout of them). tier=\"extractor\" answers one focused question with no further delegation; tier=\"verifier\" adversarially re-checks one specific claim on a stronger model (use it for absence claims, contradictions between workers, and audit-critical positives), also no further delegation; tier=\"executor\" owns a slice of work end to end and may itself delegate one level further (delegate_max_depth, default 2 — past it the tool is simply absent). Returns the workers' structured findings, never a transcript. Every worker receives the same brief and context; fanout is how one call becomes many workers, each differing only in its own fanout item. wait_sec blocks inline for the result (max 240s; extractor/verifier default 120); wait_sec 0 (executor default) starts async and returns a delegation_id — collect it with ONE get_delegation call passing wait_sec, never by re-polling in a loop. Under a CLI console engine a wait over ~120s may return as a background-task notification carrying a non-terminal status — that notification does not consume the delegation: call get_delegation once more after it, and never treat the backgrounding as an error.",
		InputSchema: json.RawMessage(`{
"type":"object",
"properties":{
"tier":{"type":"string","enum":["extractor","verifier","executor"],"description":"extractor = single-question one-shot; verifier = adversarial one-claim re-check on a stronger model; executor = owns a slice, may delegate one level further"},
"brief":{"type":"string","description":"The shared task statement every worker receives"},
"context":{"type":"string","description":"Inline context shared by all workers, capped at 4096 bytes — over the cap the call is rejected (put bulk content in an artifact and name it in artifacts instead)"},
"artifacts":{"type":"array","items":{"type":"string"},"description":"Names of artifacts already materialized on this run, passed to workers as a which-to-read hint"},
"wait_sec":{"type":"integer","description":"Block up to this many seconds (max 240) for the result; 0 starts async. Defaults: extractor 120 (inline), executor 0 (async)"},
"fanout":{"type":"array","items":{"type":"string"},"description":"One worker is spawned per item, concurrently; the item text reaches only that worker (its per-worker slice of the job, e.g. a file path or subtopic). Omit for a single worker. Capped by delegate_max_fanout (default 20); over the cap the call is rejected"}
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
		WaitSec   *int     `json:"wait_sec"`
		Fanout    []string `json:"fanout"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return invalidArgs(err)
	}
	if env.Delegator == nil {
		return missingService("delegator")
	}
	if args.Tier != "extractor" && args.Tier != "verifier" && args.Tier != "executor" {
		return `tier must be "extractor", "verifier" or "executor"`, true, nil
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

	waitSec := 0
	if args.WaitSec != nil {
		waitSec = *args.WaitSec
	} else if args.Tier != "executor" {
		waitSec = delegateDefaultInlineWaitSec
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
	if waitSec <= 0 {
		return appendPollHint(result), false, nil
	}
	delegationID := delegationID(result)
	if delegationID == "" {
		return result, false, nil
	}
	return pollDelegation(ctx, env, delegationID, waitSec)
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
