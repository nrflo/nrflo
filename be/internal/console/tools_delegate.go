package console

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"be/internal/service"
	"be/internal/spawner/apirun"
	"be/internal/spawner/apirun/provider"
)

// delegateHandler exposes delegate to console (T0) sessions. Unlike every
// other session-bound tool, delegate is intentionally reused here: it
// creates its own hidden host workflow instance when the caller has none
// (see spawner.Spawner.Delegate) — see CLAUDE.md's Profile Invariant note.
type delegateHandler struct{ d Deps }

func (delegateHandler) Spec() provider.ToolSpec {
	return provider.ToolSpec{
		Name:        "delegate",
		Description: "Delegate work downward to a cheaper tier worker (or a fanout of them). tier=\"extractor\" answers one focused question with no further delegation; tier=\"executor\" owns a slice of work end to end and may itself delegate one level further (delegate_max_depth, default 2 — past it the tool is simply absent). Starts async and returns {delegation_id, status:\"running\"} — poll with get_delegation. Every worker receives the same brief and context; fanout is how one call becomes many workers, each differing only in its own fanout item.",
		InputSchema: json.RawMessage(`{
"type":"object",
"properties":{
"tier":{"type":"string","enum":["extractor","executor"]},
"brief":{"type":"string","description":"The shared task statement every worker receives"},
"context":{"type":"string","description":"Inline context shared by all workers, capped at 4096 bytes — over the cap the call is rejected (put bulk content in an artifact and name it in artifacts instead)"},
"artifacts":{"type":"array","items":{"type":"string"},"description":"Names of artifacts already materialized on the run the workers spawn under, passed to workers as a which-to-read hint. Rarely useful from a console session: its workers run under a fresh hidden instance that starts with no artifacts"},
"fanout":{"type":"array","items":{"type":"string"},"description":"One worker is spawned per item, concurrently; the item text reaches only that worker (its per-worker slice of the job, e.g. a file path or subtopic). Omit for a single worker. Capped by delegate_max_fanout (default 20); over the cap the call is rejected"}
},
"required":["tier","brief"],
"additionalProperties":false
}`),
	}
}

func (h delegateHandler) Invoke(ctx context.Context, env apirun.ToolEnv, input json.RawMessage) (string, bool, error) {
	var args struct {
		Tier      string   `json:"tier"`
		Brief     string   `json:"brief"`
		Context   string   `json:"context"`
		Artifacts []string `json:"artifacts"`
		Fanout    []string `json:"fanout"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return invalidArgs(err)
	}
	if h.d.Delegator == nil {
		return missingService("delegator")
	}
	if args.Tier != "extractor" && args.Tier != "executor" {
		return `tier must be "extractor" or "executor"`, true, nil
	}
	if strings.TrimSpace(args.Brief) == "" {
		return "brief is required", true, nil
	}
	if len(args.Context) > 4096 {
		return "context exceeds 4096 bytes; materialize an artifact instead", true, nil
	}
	if maxFanout := service.DelegateMaxFanout(env.Pool, env.ProjectID); len(args.Fanout) > maxFanout {
		return fmt.Sprintf("fanout of %d exceeds delegate_max_fanout (%d)", len(args.Fanout), maxFanout), true, nil
	}

	result, err := h.d.Delegator.Delegate(ctx, env.SessionID, apirun.DelegateRequest{
		Tier:      args.Tier,
		Brief:     args.Brief,
		Context:   args.Context,
		Artifacts: args.Artifacts,
		Fanout:    args.Fanout,
	})
	if err != nil {
		return err.Error(), true, nil
	}
	return result, false, nil
}

// getDelegationHandler exposes get_delegation to console sessions, mirroring
// delegateHandler.
type getDelegationHandler struct{ d Deps }

func (getDelegationHandler) Spec() provider.ToolSpec {
	return provider.ToolSpec{
		Name:        "get_delegation",
		Description: "Poll an async delegation started via delegate. Returns {delegation_id, status, results?}: status running while any worker still runs (results list per-worker progress), then completed — or failed when at least one worker failed — with results[{session_id, status, reason?, findings?}] where findings is each worker's structured output. READ-ONCE: the terminal response is consumed as it is returned — worker findings and the delegation record are deleted — so store the results; polling the same delegation_id again returns an unknown-delegation error, not a repeat of the data.",
		InputSchema: json.RawMessage(`{
"type":"object",
"properties":{
"delegation_id":{"type":"string"}
},
"required":["delegation_id"],
"additionalProperties":false
}`),
	}
}

func (h getDelegationHandler) Invoke(ctx context.Context, env apirun.ToolEnv, input json.RawMessage) (string, bool, error) {
	var args struct {
		DelegationID string `json:"delegation_id"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return invalidArgs(err)
	}
	if h.d.Delegator == nil {
		return missingService("delegator")
	}
	if strings.TrimSpace(args.DelegationID) == "" {
		return "delegation_id is required", true, nil
	}

	result, err := h.d.Delegator.GetDelegation(ctx, env.SessionID, args.DelegationID)
	if err != nil {
		return err.Error(), true, nil
	}
	return result, false, nil
}
