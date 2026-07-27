package orchestrator

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/model"
	"be/internal/repo"
)

// seedChildInstance marks the instance as a sub-run of parentID and seeds a
// terminal session plus a session-scoped result finding under key, mirroring
// what emit_findings stores during a real run.
func seedChildInstance(t *testing.T, env *testEnv, wfiID, parentID, status string, key string) {
	t.Helper()
	if _, err := env.pool.Exec(
		`UPDATE workflow_instances SET status=?, launch_depth=1, parent_instance_id=?, subworkflow_depth=1 WHERE id=?`,
		status, parentID, wfiID); err != nil {
		t.Fatalf("mark instance: %v", err)
	}
	if key == "" {
		return
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := env.pool.Exec(`
		INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type, status, prompt, config, ended_at, created_at, updated_at)
		VALUES ('sw-synth', ?, '', ?, 'synthesize', 'synthesize', 'project_completed', '', '', ?, ?, ?)`,
		env.project, wfiID, now, now, now); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	if err := repo.NewFindingRepo(env.pool, clock.Real()).Upsert(
		"session", "sw-synth", key, json.RawMessage(`{"summary":"the answer"}`),
		repo.Denorm{ProjectID: env.project, WorkflowInstanceID: wfiID},
		repo.Actor{ID: "sw-synth", Source: "agent"}); err != nil {
		t.Fatalf("seed result finding: %v", err)
	}
}

// GetSubworkflow must read the result from session scope (the scope
// emit_findings actually writes), not workflow_instance scope.
func TestGetSubworkflow_ReadsSessionScopedResult(t *testing.T) {
	env := newTestEnv(t)
	wfiID := env.initProjectWorkflow(t, "test")
	seedChildInstance(t, env, wfiID, "parent-1", "project_completed", "report")

	state, err := env.orch.GetSubworkflow(context.Background(), "parent-1", env.project, wfiID, "report")
	if err != nil {
		t.Fatalf("GetSubworkflow: %v", err)
	}
	if state.Status != "completed" || !strings.Contains(string(state.Result), "the answer") {
		t.Errorf("got (%q, %s), want completed with seeded result", state.Status, state.Result)
	}
}

func TestGetSubworkflow_StatusMapping(t *testing.T) {
	env := newTestEnv(t)

	active := env.initProjectWorkflow(t, "test")
	seedChildInstance(t, env, active, "parent-1", "active", "")
	if state, err := env.orch.GetSubworkflow(context.Background(), "parent-1", env.project, active, ""); err != nil || state.Status != "running" {
		t.Errorf("active: got (%q, %v), want running", state.Status, err)
	}

	waiting := env.initProjectWorkflow(t, "test")
	seedChildInstance(t, env, waiting, "parent-1", "waiting", "")
	state, err := env.orch.GetSubworkflow(context.Background(), "parent-1", env.project, waiting, "")
	if err != nil || state.Status != "waiting" || !strings.Contains(state.FailureReason, "paused") {
		t.Errorf("waiting: got (%q, %q, %v), want waiting with pause note", state.Status, state.FailureReason, err)
	}

	failed := env.initProjectWorkflow(t, "test")
	seedChildInstance(t, env, failed, "parent-1", "failed", "")
	fr := repo.NewFindingRepo(env.pool, clock.Real())
	_ = fr.Upsert("workflow_instance", failed, "_failure_reason", json.RawMessage(`{"reason":"boom"}`),
		repo.Denorm{ProjectID: env.project, WorkflowInstanceID: failed}, repo.Actor{Source: "orchestrator"})
	state, err = env.orch.GetSubworkflow(context.Background(), "parent-1", env.project, failed, "")
	if err != nil || state.Status != "failed" || state.FailureReason != "boom" {
		t.Errorf("failed: got (%q, %q, %v), want (failed, boom)", state.Status, state.FailureReason, err)
	}
}

// Only the run that started a child may poll it: top-level runs, foreign
// projects, and foreign callers are all rejected.
func TestGetSubworkflow_ParentageAuthorization(t *testing.T) {
	env := newTestEnv(t)
	wfiID := env.initProjectWorkflow(t, "test") // parent_instance_id stays ""

	if _, err := env.orch.GetSubworkflow(context.Background(), "anyone", env.project, wfiID, ""); err == nil {
		t.Error("want error for top-level (no-parent) instance")
	}

	seedChildInstance(t, env, wfiID, "parent-1", "active", "")
	if _, err := env.orch.GetSubworkflow(context.Background(), "parent-1", "other-project", wfiID, ""); err == nil {
		t.Error("want error for foreign project")
	}
	if _, err := env.orch.GetSubworkflow(context.Background(), "other-caller", env.project, wfiID, ""); err == nil {
		t.Error("want error for a caller that did not start the child")
	}
}

// StartSubworkflow guard order: def guards first, then the persisted budget,
// then the live-parent requirement (with the budget refunded on failure).
func TestStartSubworkflow_Guards(t *testing.T) {
	env := newTestEnv(t)
	parentID := env.initProjectWorkflow(t, "test")

	// Not callable.
	_, err := env.orch.StartSubworkflow(context.Background(), parentID, env.project, "test", "go")
	if err == nil || !strings.Contains(err.Error(), "not callable_as_subworkflow") {
		t.Fatalf("want not-callable error, got %v", err)
	}

	// Callable but purging.
	if _, err := env.pool.Exec(
		`UPDATE workflows SET callable_as_subworkflow=1, purge_on_completion=1 WHERE LOWER(project_id)=LOWER(?) AND LOWER(id)='test'`,
		env.project); err != nil {
		t.Fatal(err)
	}
	_, err = env.orch.StartSubworkflow(context.Background(), parentID, env.project, "test", "go")
	if err == nil || !strings.Contains(err.Error(), "purge_on_completion") {
		t.Fatalf("want purge error, got %v", err)
	}

	// Callable, non-purging, but sub-workflow depth cap exceeded.
	if _, err := env.pool.Exec(
		`UPDATE workflows SET purge_on_completion=0 WHERE LOWER(project_id)=LOWER(?) AND LOWER(id)='test'`, env.project); err != nil {
		t.Fatal(err)
	}
	if _, err := env.pool.Exec(
		`UPDATE workflow_instances SET subworkflow_depth=3 WHERE id=?`, parentID); err != nil {
		t.Fatal(err)
	}
	_, err = env.orch.StartSubworkflow(context.Background(), parentID, env.project, "test", "go")
	if err == nil || !strings.Contains(err.Error(), "subworkflow_max_depth") {
		t.Fatalf("want depth-cap error, got %v", err)
	}

	// Persisted invocation budget exhausted (survives runState recreation).
	if _, err := env.pool.Exec(
		`UPDATE workflow_instances SET subworkflow_depth=0, subworkflow_starts=25 WHERE id=?`, parentID); err != nil {
		t.Fatal(err)
	}
	_, err = env.orch.StartSubworkflow(context.Background(), parentID, env.project, "test", "go")
	if err == nil || !strings.Contains(err.Error(), "budget exhausted") {
		t.Fatalf("want budget error, got %v", err)
	}

	// Budget ok but parent run not active: charged then refunded.
	if _, err := env.pool.Exec(
		`UPDATE workflow_instances SET subworkflow_starts=0 WHERE id=?`, parentID); err != nil {
		t.Fatal(err)
	}
	_, err = env.orch.StartSubworkflow(context.Background(), parentID, env.project, "test", "go")
	if err == nil || !strings.Contains(err.Error(), "not active") {
		t.Fatalf("want parent-not-active error, got %v", err)
	}
	var starts int
	if err := env.pool.QueryRow(`SELECT subworkflow_starts FROM workflow_instances WHERE id=?`, parentID).Scan(&starts); err != nil {
		t.Fatal(err)
	}
	if starts != 0 {
		t.Errorf("budget not refunded after failed start: %d, want 0", starts)
	}
}

// TestGetSubworkflow_PlanSuspendedStatuses_ReturnNonTerminalPollState: the
// four plan-boundary statuses must surface as their own poll states (not the
// dead-end pause_after "waiting"), so a caller's poll loop can distinguish
// them and drive the plan lifecycle instead of spinning forever.
func TestGetSubworkflow_PlanSuspendedStatuses_ReturnNonTerminalPollState(t *testing.T) {
	env := newTestEnv(t)

	statuses := []model.WorkflowInstanceStatus{
		model.WorkflowInstancePlanning,
		model.WorkflowInstanceWaitingInput,
		model.WorkflowInstanceWaitingApproval,
	}
	for _, st := range statuses {
		t.Run(string(st), func(t *testing.T) {
			wfiID := env.initProjectWorkflow(t, "test")
			seedChildInstance(t, env, wfiID, "parent-1", string(st), "")

			state, err := env.orch.GetSubworkflow(context.Background(), "parent-1", env.project, wfiID, "")
			if err != nil {
				t.Fatalf("GetSubworkflow: %v", err)
			}
			if state.Status != string(st) {
				t.Errorf("status = %q, want %q", state.Status, st)
			}
			if state.Result != nil {
				t.Errorf("result = %s, want nil", state.Result)
			}
			if state.FailureReason != "" {
				t.Errorf("failureReason = %q, want empty for a plan-boundary status (no result/failure payload)", state.FailureReason)
			}
		})
	}
}
