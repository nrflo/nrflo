package console

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"be/internal/repo"
)

// TestWorkflowWait_TimeoutHasNoStateKey asserts the timeout envelope
// ({changed:false, terminal:false, digest}) never carries a "state" key at
// all — not even null — distinguishing "nothing happened" from a real
// (possibly empty) tracking snapshot.
func TestWorkflowWait_TimeoutHasNoStateKey(t *testing.T) {
	env := newConsoleTestEnv(t)
	env.seedWorkflowInstance(t, testProjectID, "wfi-wait-nostate")
	reg, _ := BuildRegistry(env.deps, nil)
	toolEnv := NewToolEnv(env.deps, "sess-1", testProjectID)

	base, _, _ := invoke(t, reg, toolEnv, "workflow_wait", `{"instance_id":"wfi-wait-nostate"}`)
	baseline := parseWaitResult(t, base)

	input := fmt.Sprintf(`{"instance_id":"wfi-wait-nostate","since_digest":%q,"timeout_seconds":55}`, baseline.Digest)
	outcome := invokeWaitAsync(context.Background(), reg, toolEnv, input)
	awaitWaiter(t, env.deps.WaitBroker, testProjectID)

	env.clk.Advance(56 * time.Second)

	o := receiveOutcome(t, outcome)
	if o.err != nil || o.isErr {
		t.Fatalf("err=%v isErr=%v out=%s", o.err, o.isErr, o.out)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(o.out), &raw); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, o.out)
	}
	if _, ok := raw["state"]; ok {
		t.Errorf("timeout response has a %q key, want none: %s", "state", o.out)
	}
	for _, k := range []string{"changed", "terminal", "digest"} {
		if _, ok := raw[k]; !ok {
			t.Errorf("timeout response missing %q key: %s", k, o.out)
		}
	}
}

// TestWorkflowWait_ChangedResponseOmitsTopologyAndFullState asserts a
// changed-but-non-terminal transition (since_digest set, i.e. not baseline)
// carries the tracking subset only: status/current_phase/phases/active_agents
// present, findings/agent_history/phase_order/phase_layers/layer_policies/
// workflow absent (topology is baseline-only; the rest is workflow_get's job).
func TestWorkflowWait_ChangedResponseOmitsTopologyAndFullState(t *testing.T) {
	env := newConsoleTestEnv(t)
	env.seedWorkflowInstance(t, testProjectID, "wfi-wait-chg")
	reg, _ := BuildRegistry(env.deps, nil)
	toolEnv := NewToolEnv(env.deps, "sess-1", testProjectID)

	base, _, _ := invoke(t, reg, toolEnv, "workflow_wait", `{"instance_id":"wfi-wait-chg"}`)
	baseline := parseWaitResult(t, base)
	if _, ok := baseline.State["phase_order"]; !ok {
		t.Fatalf("baseline state missing phase_order: %+v", baseline.State)
	}

	mustExec(t, env.pool, `UPDATE workflow_instances SET status='waiting' WHERE id='wfi-wait-chg'`)

	input := fmt.Sprintf(`{"instance_id":"wfi-wait-chg","since_digest":%q}`, baseline.Digest)
	out, isErr, err := invoke(t, reg, toolEnv, "workflow_wait", input)
	if err != nil || isErr {
		t.Fatalf("err=%v isErr=%v out=%s", err, isErr, out)
	}
	r := parseWaitResult(t, out)
	if !r.Changed || r.Terminal {
		t.Fatalf("result = %+v, want changed=true terminal=false", r)
	}
	if r.State["status"] != "waiting" {
		t.Errorf("state.status = %v, want waiting", r.State["status"])
	}
	if _, ok := r.State["current_phase"]; !ok {
		t.Errorf("state missing current_phase: %+v", r.State)
	}
	if _, ok := r.State["phases"]; !ok {
		t.Errorf("state missing phases: %+v", r.State)
	}
	if _, ok := r.State["active_agents"]; !ok {
		t.Errorf("state missing active_agents: %+v", r.State)
	}
	for _, k := range []string{"findings", "agent_history", "phase_order", "phase_layers", "layer_policies", "workflow", "instance_id"} {
		if _, ok := r.State[k]; ok {
			t.Errorf("non-baseline changed state carries %q, want absent: %+v", k, r.State)
		}
	}
}

// TestWorkflowWait_TerminalCompletedSurfacesFinalResult seeds a terminal
// completed instance with a workflow_final_result finding and asserts it
// is copied through to state.workflow_final_result.
func TestWorkflowWait_TerminalCompletedSurfacesFinalResult(t *testing.T) {
	env := newConsoleTestEnv(t)
	env.seedWorkflowInstance(t, testProjectID, "wfi-wait-done2")
	now := env.clk.Now().UTC().Format(time.RFC3339Nano)
	mustExec(t, env.pool, `INSERT INTO agent_sessions
		(id, project_id, ticket_id, workflow_instance_id, phase, node_id, agent_type, status, ended_at, created_at, updated_at)
		VALUES ('sess-done2', ?, '', 'wfi-wait-done2', 'review', 'review', 'implementor', 'completed', ?, ?, ?)`,
		testProjectID, now, now, now)
	fr := repo.NewFindingRepo(env.pool, env.clk)
	if err := fr.Upsert("session", "sess-done2", "workflow_final_result", json.RawMessage(`"all good"`),
		repo.Denorm{ProjectID: testProjectID, WorkflowInstanceID: "wfi-wait-done2"}, repo.Actor{Source: "test"}); err != nil {
		t.Fatalf("seed finding: %v", err)
	}
	mustExec(t, env.pool, `UPDATE workflow_instances SET status='completed' WHERE id='wfi-wait-done2'`)
	reg, _ := BuildRegistry(env.deps, nil)
	toolEnv := NewToolEnv(env.deps, "sess-1", testProjectID)

	out, isErr, err := invoke(t, reg, toolEnv, "workflow_wait", `{"instance_id":"wfi-wait-done2"}`)
	if err != nil || isErr {
		t.Fatalf("err=%v isErr=%v out=%s", err, isErr, out)
	}
	r := parseWaitResult(t, out)
	if !r.Terminal {
		t.Fatalf("terminal = false, want true")
	}
	if r.State["workflow_final_result"] != "all good" {
		t.Errorf("state.workflow_final_result = %v, want %q", r.State["workflow_final_result"], "all good")
	}
	if _, ok := r.State["findings"]; ok {
		t.Errorf("terminal state carries findings, want absent: %+v", r.State)
	}
}

// TestWorkflowWait_TerminalFailedSurfacesFailureReason seeds a failed
// instance with a `_failure_reason` finding and asserts failure_reason is
// surfaced at the top level (not inside state) with no findings map leaking.
func TestWorkflowWait_TerminalFailedSurfacesFailureReason(t *testing.T) {
	env := newConsoleTestEnv(t)
	env.seedWorkflowInstance(t, testProjectID, "wfi-wait-failed")
	fr := repo.NewFindingRepo(env.pool, env.clk)
	if err := fr.Upsert("workflow_instance", "wfi-wait-failed", "_failure_reason", json.RawMessage(`{"reason":"boom"}`),
		repo.Denorm{ProjectID: testProjectID, WorkflowInstanceID: "wfi-wait-failed"}, repo.Actor{Source: "test"}); err != nil {
		t.Fatalf("seed finding: %v", err)
	}
	mustExec(t, env.pool, `UPDATE workflow_instances SET status='failed' WHERE id='wfi-wait-failed'`)
	reg, _ := BuildRegistry(env.deps, nil)
	toolEnv := NewToolEnv(env.deps, "sess-1", testProjectID)

	out, isErr, err := invoke(t, reg, toolEnv, "workflow_wait", `{"instance_id":"wfi-wait-failed"}`)
	if err != nil || isErr {
		t.Fatalf("err=%v isErr=%v out=%s", err, isErr, out)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, out)
	}
	var failureReason string
	if err := json.Unmarshal(raw["failure_reason"], &failureReason); err != nil {
		t.Fatalf("unmarshal failure_reason: %v\n%s", err, out)
	}
	if failureReason != "boom" {
		t.Errorf("failure_reason = %q, want boom", failureReason)
	}
	r := parseWaitResult(t, out)
	if _, ok := r.State["findings"]; ok {
		t.Errorf("terminal-failed state carries findings, want absent: %+v", r.State)
	}
}

// TestWorkflowWait_ActiveAgentsTrimmedToTrackingFields seeds an
// agent_sessions row with extra columns beyond the tracking subset and
// asserts state.active_agents entries expose only
// node_id/agent_type/model_id/restart_count.
func TestWorkflowWait_ActiveAgentsTrimmedToTrackingFields(t *testing.T) {
	env := newConsoleTestEnv(t)
	env.seedWorkflowInstance(t, testProjectID, "wfi-wait-agents")
	now := env.clk.Now().UTC().Format(time.RFC3339Nano)
	mustExec(t, env.pool, `INSERT INTO agent_sessions
		(id, project_id, ticket_id, workflow_instance_id, phase, node_id, agent_type, model_id, status, pid, context_left, restart_count, created_at, updated_at)
		VALUES ('sess-agents', ?, '', 'wfi-wait-agents', 'review', 'review', 'implementor', 'anthropic:claude', 'running', 4242, 80, 2, ?, ?)`,
		testProjectID, now, now)
	reg, _ := BuildRegistry(env.deps, nil)
	toolEnv := NewToolEnv(env.deps, "sess-1", testProjectID)

	out, isErr, err := invoke(t, reg, toolEnv, "workflow_wait", `{"instance_id":"wfi-wait-agents"}`)
	if err != nil || isErr {
		t.Fatalf("err=%v isErr=%v out=%s", err, isErr, out)
	}
	r := parseWaitResult(t, out)
	agents, ok := r.State["active_agents"].(map[string]interface{})
	if !ok || len(agents) != 1 {
		t.Fatalf("active_agents = %+v, want exactly one entry", r.State["active_agents"])
	}
	var entry map[string]interface{}
	for _, v := range agents {
		entry, _ = v.(map[string]interface{})
	}
	if entry == nil {
		t.Fatalf("active_agents entry is not an object: %+v", agents)
	}
	wantKeys := map[string]interface{}{
		"node_id":       "review",
		"agent_type":    "implementor",
		"model_id":      "anthropic:claude",
		"restart_count": float64(2),
	}
	for k, want := range wantKeys {
		if got := entry[k]; got != want {
			t.Errorf("active_agents entry[%q] = %v, want %v", k, got, want)
		}
	}
	for _, leak := range []string{"pid", "context_left", "session_id", "agent_id", "result", "phase"} {
		if _, ok := entry[leak]; ok {
			t.Errorf("active_agents entry leaks untracked key %q: %+v", leak, entry)
		}
	}
}
