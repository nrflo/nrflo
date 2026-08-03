package console

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"

	"be/internal/model"
	"be/internal/spawner/apirun"
	"be/internal/ws"
)

type waitResult struct {
	Changed               bool                   `json:"changed"`
	Terminal              bool                   `json:"terminal"`
	Digest                string                 `json:"digest"`
	State                 map[string]interface{} `json:"state"`
	NextWorkflowOnSuccess string                 `json:"next_workflow_on_success"`
}

type invokeOutcome struct {
	out   string
	isErr bool
	err   error
}

func parseWaitResult(t *testing.T, out string) waitResult {
	t.Helper()
	var r waitResult
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatalf("wait output does not unmarshal: %v\n%s", err, out)
	}
	return r
}

// invokeWaitAsync runs workflow_wait in a goroutine and returns its outcome
// channel; awaitWaiter spins (no sleep) until the handler has subscribed.
func invokeWaitAsync(ctx context.Context, reg apirun.Registry, env apirun.ToolEnv, input string) <-chan invokeOutcome {
	ch := make(chan invokeOutcome, 1)
	go func() {
		out, isErr, err := reg["workflow_wait"].Invoke(ctx, env, json.RawMessage(input))
		ch <- invokeOutcome{out, isErr, err}
	}()
	return ch
}

func awaitWaiter(t *testing.T, b *WaitBroker, projectID string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for b.WaiterCount(projectID) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("workflow_wait never subscribed a waiter")
		}
		runtime.Gosched()
	}
}

func receiveOutcome(t *testing.T, ch <-chan invokeOutcome) invokeOutcome {
	t.Helper()
	select {
	case o := <-ch:
		return o
	case <-time.After(10 * time.Second):
		t.Fatal("workflow_wait did not return")
		return invokeOutcome{}
	}
}

func TestWorkflowWait_BaselineReturnsImmediately(t *testing.T) {
	env := newConsoleTestEnv(t)
	env.seedWorkflowInstance(t, testProjectID, "wfi-wait-base")
	reg, err := BuildRegistry(env.deps, nil)
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}
	toolEnv := NewToolEnv(env.deps, "sess-1", testProjectID, model.AgentSessionKindConsole)

	out, isErr, err := invoke(t, reg, toolEnv, "workflow_wait", `{"instance_id":"wfi-wait-base"}`)
	if err != nil || isErr {
		t.Fatalf("err=%v isErr=%v out=%s", err, isErr, out)
	}
	r := parseWaitResult(t, out)
	if !r.Changed || r.Terminal || r.Digest == "" {
		t.Errorf("baseline = %+v, want changed=true terminal=false non-empty digest", r)
	}
	if r.State["status"] != "active" {
		t.Errorf("state.status = %v, want active", r.State["status"])
	}
	if _, ok := r.State["instance_id"]; ok {
		t.Errorf("state carries instance_id, want absent (trimmed tracking projection): %+v", r.State)
	}
	for _, k := range []string{"phase_order", "current_phase", "phases", "active_agents"} {
		if _, ok := r.State[k]; !ok {
			t.Errorf("baseline state missing %q: %+v", k, r.State)
		}
	}
	if got := env.deps.WaitBroker.WaiterCount(testProjectID); got != 0 {
		t.Errorf("WaiterCount after return = %d, want 0 (subscription leaked)", got)
	}
}

func TestWorkflowWait_TerminalInstanceReturnsImmediately(t *testing.T) {
	env := newConsoleTestEnv(t)
	env.seedWorkflowInstance(t, testProjectID, "wfi-wait-done")
	mustExec(t, env.pool, `UPDATE workflow_instances SET status='completed' WHERE id='wfi-wait-done'`)
	reg, _ := BuildRegistry(env.deps, nil)
	toolEnv := NewToolEnv(env.deps, "sess-1", testProjectID, model.AgentSessionKindConsole)

	out, isErr, err := invoke(t, reg, toolEnv, "workflow_wait", `{"instance_id":"wfi-wait-done"}`)
	if err != nil || isErr {
		t.Fatalf("err=%v isErr=%v out=%s", err, isErr, out)
	}
	r := parseWaitResult(t, out)
	if !r.Terminal {
		t.Errorf("terminal = false, want true for completed instance")
	}
}

func TestWorkflowWait_WakesOnBroadcastAndReportsTerminal(t *testing.T) {
	env := newConsoleTestEnv(t)
	env.seedWorkflowInstance(t, testProjectID, "wfi-wait-wake")
	mustExec(t, env.pool, `UPDATE workflows SET next_workflow_on_success='triage-next' WHERE id='wf-wfi-wait-wake'`)
	reg, _ := BuildRegistry(env.deps, nil)
	toolEnv := NewToolEnv(env.deps, "sess-1", testProjectID, model.AgentSessionKindConsole)

	base, _, _ := invoke(t, reg, toolEnv, "workflow_wait", `{"instance_id":"wfi-wait-wake"}`)
	baseline := parseWaitResult(t, base)

	input := fmt.Sprintf(`{"instance_id":"wfi-wait-wake","since_digest":%q,"timeout_seconds":600}`, baseline.Digest)
	outcome := invokeWaitAsync(context.Background(), reg, toolEnv, input)
	awaitWaiter(t, env.deps.WaitBroker, testProjectID)

	mustExec(t, env.pool, `UPDATE workflow_instances SET status='completed' WHERE id='wfi-wait-wake'`)
	env.deps.WaitBroker.OnEvent(&ws.Event{ProjectID: testProjectID, Type: ws.EventWorkflowUpdated})

	o := receiveOutcome(t, outcome)
	if o.err != nil || o.isErr {
		t.Fatalf("err=%v isErr=%v out=%s", o.err, o.isErr, o.out)
	}
	r := parseWaitResult(t, o.out)
	if !r.Changed || !r.Terminal {
		t.Errorf("result = %+v, want changed=true terminal=true", r)
	}
	if r.Digest == baseline.Digest {
		t.Errorf("digest did not change across a status transition")
	}
	if r.NextWorkflowOnSuccess != "triage-next" {
		t.Errorf("next_workflow_on_success = %q, want triage-next", r.NextWorkflowOnSuccess)
	}
}

func TestWorkflowWait_TimeoutReturnsUnchanged(t *testing.T) {
	env := newConsoleTestEnv(t)
	env.seedWorkflowInstance(t, testProjectID, "wfi-wait-to")
	reg, _ := BuildRegistry(env.deps, nil)
	toolEnv := NewToolEnv(env.deps, "sess-1", testProjectID, model.AgentSessionKindConsole)

	base, _, _ := invoke(t, reg, toolEnv, "workflow_wait", `{"instance_id":"wfi-wait-to"}`)
	baseline := parseWaitResult(t, base)

	input := fmt.Sprintf(`{"instance_id":"wfi-wait-to","since_digest":%q,"timeout_seconds":55}`, baseline.Digest)
	outcome := invokeWaitAsync(context.Background(), reg, toolEnv, input)
	awaitWaiter(t, env.deps.WaitBroker, testProjectID)

	env.clk.Advance(56 * time.Second)

	o := receiveOutcome(t, outcome)
	if o.err != nil || o.isErr {
		t.Fatalf("err=%v isErr=%v out=%s", o.err, o.isErr, o.out)
	}
	r := parseWaitResult(t, o.out)
	if r.Changed || r.Terminal {
		t.Errorf("result = %+v, want changed=false terminal=false on timeout", r)
	}
	if r.Digest != baseline.Digest {
		t.Errorf("digest changed on timeout: %q -> %q", baseline.Digest, r.Digest)
	}
}

func TestWorkflowWait_ContextCancelReturnsError(t *testing.T) {
	env := newConsoleTestEnv(t)
	env.seedWorkflowInstance(t, testProjectID, "wfi-wait-cancel")
	reg, _ := BuildRegistry(env.deps, nil)
	toolEnv := NewToolEnv(env.deps, "sess-1", testProjectID, model.AgentSessionKindConsole)

	base, _, _ := invoke(t, reg, toolEnv, "workflow_wait", `{"instance_id":"wfi-wait-cancel"}`)
	baseline := parseWaitResult(t, base)

	ctx, cancel := context.WithCancel(context.Background())
	input := fmt.Sprintf(`{"instance_id":"wfi-wait-cancel","since_digest":%q}`, baseline.Digest)
	outcome := invokeWaitAsync(ctx, reg, toolEnv, input)
	awaitWaiter(t, env.deps.WaitBroker, testProjectID)
	cancel()

	o := receiveOutcome(t, outcome)
	if o.err != nil {
		t.Fatalf("err = %v, want nil (cancellation is a tool-level error)", o.err)
	}
	if !o.isErr || !strings.Contains(o.out, "cancelled") {
		t.Errorf("out=%q isErr=%v, want cancelled tool error", o.out, o.isErr)
	}
}

func TestWorkflowWait_CrossProjectInstanceID_Rejected(t *testing.T) {
	env := newConsoleTestEnv(t)
	env.seedWorkflowInstance(t, testOtherProjectID, "wfi-wait-other")
	reg, _ := BuildRegistry(env.deps, nil)
	toolEnv := NewToolEnv(env.deps, "sess-1", testProjectID, model.AgentSessionKindConsole)

	out, isErr, err := invoke(t, reg, toolEnv, "workflow_wait", `{"instance_id":"wfi-wait-other"}`)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if !isErr {
		t.Errorf("isErr = false, want true for cross-project instance_id; out=%s", out)
	}
}

func TestComputeWaitDigest_TransitionSensitivity(t *testing.T) {
	base := map[string]interface{}{
		"status":        "active",
		"current_phase": "review",
		"phases": map[string]model.PhaseStatus{
			"review": {Status: "in_progress"},
		},
		"active_agents": map[string]interface{}{
			"review": map[string]interface{}{"session_id": "s1", "context_left": int64(80)},
		},
	}
	d1 := computeWaitDigest(base)
	if d1 != computeWaitDigest(base) {
		t.Fatal("digest not deterministic")
	}

	statusChanged := map[string]interface{}{
		"status":        "completed",
		"current_phase": "review",
		"phases": map[string]model.PhaseStatus{
			"review": {Status: "completed"},
		},
		"active_agents": map[string]interface{}{},
	}
	if computeWaitDigest(statusChanged) == d1 {
		t.Error("digest unchanged across status/phase transition")
	}

	// Volatile telemetry must NOT move the digest.
	telemetryOnly := map[string]interface{}{
		"status":        "active",
		"current_phase": "review",
		"phases": map[string]model.PhaseStatus{
			"review": {Status: "in_progress"},
		},
		"active_agents": map[string]interface{}{
			"review": map[string]interface{}{"session_id": "s1", "context_left": int64(20)},
		},
	}
	if computeWaitDigest(telemetryOnly) != d1 {
		t.Error("digest moved on context_left change; waits would return on telemetry chatter")
	}

	// An agent restart (new session id, same key) must move it.
	restarted := map[string]interface{}{
		"status":        "active",
		"current_phase": "review",
		"phases": map[string]model.PhaseStatus{
			"review": {Status: "in_progress"},
		},
		"active_agents": map[string]interface{}{
			"review": map[string]interface{}{"session_id": "s2", "context_left": int64(80)},
		},
	}
	if computeWaitDigest(restarted) == d1 {
		t.Error("digest unchanged across agent restart (session id change)")
	}
}
