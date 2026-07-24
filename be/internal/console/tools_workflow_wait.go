package console

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"be/internal/model"
	"be/internal/spawner/apirun"
	"be/internal/spawner/apirun/provider"
)

const (
	waitDefaultTimeoutSec = 55
	waitMaxTimeoutSec     = 600
)

// workflowWaitHandler implements workflow_wait: a long-poll over the same v4
// state workflow_get returns. It blocks until the instance's state digest
// differs from since_digest (woken by WaitBroker on hub broadcasts) or the
// timeout elapses — one tool call per transition instead of per poll.
type workflowWaitHandler struct{ d Deps }

func (workflowWaitHandler) Spec() provider.ToolSpec {
	return provider.ToolSpec{
		Name:        "workflow_wait",
		Description: "Long-poll a workflow instance for progress. Blocks until the instance's state digest (status, current phase, per-phase statuses, active agent sessions, plan status) differs from since_digest or timeout_seconds elapses, then returns {changed, terminal, digest, state?} — a trimmed tracking projection (status, current_phase, phases[status+result], active_agents[node_id,agent_type,model_id,restart_count], plan[status+questions]), NOT the full v4 map. A timeout returns {changed:false, terminal:false, digest} with no state key. Usage: call workflow_wait(instance_id) after workflow_run for the baseline digest (phase_order/phase_layers/layer_policies included only on this first call), then loop with since_digest=<previous digest> until terminal=true. Terminal responses add workflow_final_result and, on failure, failure_reason, plus next_workflow_on_success when the definition chains a follow-up workflow (a new project-scoped instance). Call workflow_get for the full state snapshot (findings, agent_history, etc).",
		InputSchema: json.RawMessage(`{
"type":"object",
"properties":{
"instance_id":{"type":"string"},
"since_digest":{"type":"string","description":"Digest from the previous workflow_wait response; omit for an immediate baseline snapshot"},
"timeout_seconds":{"type":"integer","description":"Max seconds to block before returning changed=false (default 55, max 600)"}
},
"required":["instance_id"],
"additionalProperties":false
}`),
	}
}

func (h workflowWaitHandler) Invoke(ctx context.Context, env apirun.ToolEnv, input json.RawMessage) (string, bool, error) {
	var args struct {
		InstanceID     string `json:"instance_id"`
		SinceDigest    string `json:"since_digest"`
		TimeoutSeconds int    `json:"timeout_seconds"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return invalidArgs(err)
	}
	if args.InstanceID == "" {
		return "instance_id is required", true, nil
	}
	if h.d.WorkflowSvc == nil {
		return missingService("workflow")
	}
	if h.d.WaitBroker == nil {
		return missingService("workflow wait")
	}
	timeout := args.TimeoutSeconds
	if timeout <= 0 {
		timeout = waitDefaultTimeoutSec
	} else if timeout > waitMaxTimeoutSec {
		timeout = waitMaxTimeoutSec
	}

	// Subscribe before the first digest read so a broadcast landing between
	// read and select is never missed (the wake channel buffers one hint).
	wake, cancel := h.d.WaitBroker.Subscribe(env.ProjectID)
	defer cancel()
	deadline := h.d.Clock.After(time.Duration(timeout) * time.Second)

	for {
		wi, err := loadGuardedInstance(h.d, env.ProjectID, args.InstanceID)
		if err != nil {
			return err.Error(), true, nil
		}
		state, err := h.d.WorkflowSvc.GetStatusByInstance(wi)
		if err != nil {
			return err.Error(), true, nil
		}
		digest := computeWaitDigest(state)
		terminal := isTerminalInstanceStatus(wi.Status)
		isBaseline := args.SinceDigest == ""
		if terminal || isBaseline || digest != args.SinceDigest {
			out, isErr := h.marshalWaitResult(wi, state, digest, digest != args.SinceDigest, terminal, isBaseline)
			return out, isErr, nil
		}
		select {
		case <-wake:
		case <-deadline:
			return marshalWaitTimeout(digest), false, nil
		case <-ctx.Done():
			return "workflow_wait cancelled: " + ctx.Err().Error(), true, nil
		}
	}
}

// marshalWaitTimeout builds the {changed:false, terminal:false, digest}
// response for a deadline expiry — no state key, since nothing changed.
func marshalWaitTimeout(digest string) string {
	resp := map[string]interface{}{
		"changed":  false,
		"terminal": false,
		"digest":   digest,
	}
	out, err := json.Marshal(resp)
	if err != nil {
		return err.Error()
	}
	return string(out)
}

// marshalWaitResult builds the {changed, terminal, digest, state} response
// for a changed or terminal transition; state is the trimmed tracking
// projection from buildTrackingState, not the full v4 map. Terminal responses
// carry next_workflow_on_success when the source definition declares one (the
// chained run starts as a NEW instance with no back-link — the name is the
// only pointer the caller gets) and failure_reason on a failed terminal.
func (h workflowWaitHandler) marshalWaitResult(wi *model.WorkflowInstance, state map[string]interface{}, digest string, changed, terminal, isBaseline bool) (string, bool) {
	resp := map[string]interface{}{
		"changed":  changed,
		"terminal": terminal,
		"digest":   digest,
		"state":    buildTrackingState(state, isBaseline),
	}
	if terminal {
		if wi.Status == model.WorkflowInstanceFailed {
			resp["failure_reason"] = h.d.WorkflowSvc.ExtractWorkflowFailureReason(wi)
		} else if def, err := h.d.WorkflowSvc.GetWorkflowDef(wi.ProjectID, wi.WorkflowID); err == nil && def.NextWorkflowOnSuccess != "" {
			resp["next_workflow_on_success"] = def.NextWorkflowOnSuccess
		}
	}
	out, err := json.Marshal(resp)
	if err != nil {
		return err.Error(), true
	}
	return string(out), false
}

// buildTrackingState projects the full v4 state down to the fields a
// long-polling caller needs to track progress: status/current_phase always,
// phases/active_agents/plan trimmed to their tracking-relevant subfields,
// topology (phase_order/phase_layers/layer_policies) only on the baseline
// call, and workflow_final_result only once the run is terminal. Pure
// function of its inputs — externally-sourced terminal fields
// (next_workflow_on_success, failure_reason) stay in marshalWaitResult.
func buildTrackingState(state map[string]interface{}, isBaseline bool) map[string]interface{} {
	out := map[string]interface{}{
		"status":        state["status"],
		"current_phase": state["current_phase"],
	}
	if phases, ok := state["phases"].(map[string]model.PhaseStatus); ok {
		trimmed := make(map[string]interface{}, len(phases))
		for k, v := range phases {
			trimmed[k] = map[string]interface{}{"status": v.Status, "result": v.Result}
		}
		out["phases"] = trimmed
	}
	if agents, ok := state["active_agents"].(map[string]interface{}); ok {
		trimmed := make(map[string]interface{}, len(agents))
		for k, v := range agents {
			if m, ok := v.(map[string]interface{}); ok {
				trimmed[k] = map[string]interface{}{
					"node_id":       m["node_id"],
					"agent_type":    m["agent_type"],
					"model_id":      m["model_id"],
					"restart_count": m["restart_count"],
				}
			}
		}
		out["active_agents"] = trimmed
	}
	if plan, ok := state["plan"].(map[string]interface{}); ok {
		out["plan"] = map[string]interface{}{"status": plan["status"], "questions": plan["questions"]}
	}
	if isBaseline {
		if v, ok := state["phase_order"]; ok {
			out["phase_order"] = v
		}
		if v, ok := state["phase_layers"]; ok {
			out["phase_layers"] = v
		}
		if v, ok := state["layer_policies"]; ok {
			out["layer_policies"] = v
		}
	}
	if v, ok := state["workflow_final_result"]; ok {
		out["workflow_final_result"] = v
	}
	return out
}

func isTerminalInstanceStatus(s model.WorkflowInstanceStatus) bool {
	switch s {
	case model.WorkflowInstanceCompleted, model.WorkflowInstanceFailed, model.WorkflowInstanceProjectCompleted:
		return true
	default:
		return false
	}
}

// computeWaitDigest hashes the transition-relevant slice of the v4 state:
// instance status, current phase, per-phase status+result, the active agent
// key→session_id set (a restart mints a new session id), and plan status.
// Volatile telemetry (context_left, token counts, nudge/restart counters) is
// deliberately excluded so waits return on transitions, not on chatter.
func computeWaitDigest(state map[string]interface{}) string {
	d := map[string]interface{}{
		"status":        state["status"],
		"current_phase": state["current_phase"],
	}
	if phases, ok := state["phases"].(map[string]model.PhaseStatus); ok {
		ps := make(map[string]string, len(phases))
		for k, v := range phases {
			ps[k] = v.Status + "|" + v.Result
		}
		d["phases"] = ps
	}
	if agents, ok := state["active_agents"].(map[string]interface{}); ok {
		as := make(map[string]string, len(agents))
		for k, v := range agents {
			if m, ok := v.(map[string]interface{}); ok {
				as[k], _ = m["session_id"].(string)
			}
		}
		d["agents"] = as
	}
	if plan, ok := state["plan"].(map[string]interface{}); ok {
		d["plan"] = plan["status"]
	}
	b, _ := json.Marshal(d) //nolint:errcheck
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:12])
}
