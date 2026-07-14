package console

import (
	"context"
	"strings"
	"testing"
	"time"

	"be/internal/repo"
	"be/internal/service"
)

// shrinkDeepResearchTimings replaces the package-level poll vars with tiny
// values for the duration of the test (root CLAUDE.md rule 4: no time.Sleep,
// no minutes-long test waits) and restores them on cleanup.
func shrinkDeepResearchTimings(t *testing.T, pollInterval, maxWait time.Duration) {
	t.Helper()
	origInterval, origWait := deepResearchPollInterval, deepResearchMaxWait
	deepResearchPollInterval = pollInterval
	deepResearchMaxWait = maxWait
	t.Cleanup(func() {
		deepResearchPollInterval = origInterval
		deepResearchMaxWait = origWait
	})
}

// seedGlobalDeepResearchInstance seeds a projects/workflows/workflow_instances
// row under service.GlobalProjectID + service.DeepResearchWorkflow (the
// scope the deep_research tool always runs in) with the given status, and
// returns its instance id.
func (e *consoleTestEnv) seedGlobalDeepResearchInstance(t *testing.T, instanceID, status string) string {
	t.Helper()
	now := e.clk.Now().UTC().Format(time.RFC3339Nano)
	mustExec(t, e.pool, `INSERT OR IGNORE INTO projects (id, name, created_at, updated_at) VALUES (?, ?, ?, ?)`,
		service.GlobalProjectID, "Global", now, now)
	mustExec(t, e.pool, `INSERT OR IGNORE INTO workflows (id, project_id, description, created_at, updated_at, scope_type, groups)
		VALUES (?, ?, '', ?, ?, 'project', '[]')`, service.DeepResearchWorkflow, service.GlobalProjectID, now, now)
	mustExec(t, e.pool, `INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, scope_type, retry_count, created_at, updated_at)
		VALUES (?, ?, '', ?, ?, 'project', 0, ?, ?)`, instanceID, service.GlobalProjectID, service.DeepResearchWorkflow, status, now, now)
	return instanceID
}

func TestDeepResearch_MissingQuestion_Errors(t *testing.T) {
	env := newConsoleTestEnv(t)
	reg, err := BuildRegistry(env.deps)
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}
	toolEnv := NewToolEnv(env.deps, "sess-1", testProjectID)

	out, isErr, err := invoke(t, reg, toolEnv, "deep_research", `{}`)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if !isErr || !strings.Contains(out, "question is required") {
		t.Errorf("out=%q isErr=%v, want question is required", out, isErr)
	}
}

func TestDeepResearch_NilOrchestrator_MissingService(t *testing.T) {
	env := newConsoleTestEnv(t)
	reg, err := BuildRegistry(env.deps)
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}
	toolEnv := NewToolEnv(env.deps, "sess-1", testProjectID)

	out, isErr, err := invoke(t, reg, toolEnv, "deep_research", `{"question":"why?"}`)
	if err != nil || !isErr || !strings.Contains(out, "orchestrator") {
		t.Errorf("err=%v isErr=%v out=%q, want orchestrator not configured", err, isErr, out)
	}
}

func TestDeepResearch_HappyPath_ReportExtracted(t *testing.T) {
	shrinkDeepResearchTimings(t, time.Millisecond, time.Minute)
	env := newConsoleTestEnv(t)
	fake := &fakeOrchestrator{startInstanceID: "wfi-dr-ok"}
	env.deps.Orch = fake
	env.seedGlobalDeepResearchInstance(t, "wfi-dr-ok", "completed")

	findingRepo := repo.NewFindingRepo(env.pool, env.clk)
	if err := findingRepo.Upsert("session", "sess-synth", "report", []byte(`"the synthesized report"`),
		repo.Denorm{WorkflowInstanceID: "wfi-dr-ok", AgentType: "synthesize"}, repo.Actor{Source: "agent"}); err != nil {
		t.Fatalf("seed report finding: %v", err)
	}

	reg, err := BuildRegistry(env.deps)
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}
	toolEnv := NewToolEnv(env.deps, "sess-1", testProjectID)

	out, isErr, err := invoke(t, reg, toolEnv, "deep_research", `{"question":"why?"}`)
	if err != nil || isErr {
		t.Fatalf("err=%v isErr=%v out=%s", err, isErr, out)
	}
	if out != "the synthesized report" {
		t.Errorf("out = %q, want the synthesized report", out)
	}
	if fake.startWorkflow != service.DeepResearchWorkflow || fake.startProjectID != service.GlobalProjectID {
		t.Errorf("fake orchestrator not called with deep-research/global scope: %+v", fake)
	}
}

func TestDeepResearch_FailedRun_Errors(t *testing.T) {
	shrinkDeepResearchTimings(t, time.Millisecond, time.Minute)
	env := newConsoleTestEnv(t)
	fake := &fakeOrchestrator{startInstanceID: "wfi-dr-fail"}
	env.deps.Orch = fake
	env.seedGlobalDeepResearchInstance(t, "wfi-dr-fail", "failed")

	reg, err := BuildRegistry(env.deps)
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}
	toolEnv := NewToolEnv(env.deps, "sess-1", testProjectID)

	out, isErr, err := invoke(t, reg, toolEnv, "deep_research", `{"question":"why?"}`)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if !isErr || !strings.Contains(out, "failed") {
		t.Errorf("out=%q isErr=%v, want failed message", out, isErr)
	}
}

func TestDeepResearch_CtxCancelled_StopsRun(t *testing.T) {
	shrinkDeepResearchTimings(t, time.Millisecond, time.Minute)
	env := newConsoleTestEnv(t)
	fake := &fakeOrchestrator{startInstanceID: "wfi-dr-cancel"}
	env.deps.Orch = fake
	env.seedGlobalDeepResearchInstance(t, "wfi-dr-cancel", "active")

	reg, err := BuildRegistry(env.deps)
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}
	toolEnv := NewToolEnv(env.deps, "sess-1", testProjectID)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	h, ok := reg["deep_research"]
	if !ok {
		t.Fatalf("deep_research not registered")
	}
	out, isErr, err := h.Invoke(ctx, toolEnv, []byte(`{"question":"why?"}`))
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if !isErr {
		t.Errorf("isErr = false, want true on ctx cancellation; out=%s", out)
	}
	if fake.stopProjectCalled != 1 || fake.stopProjectInstanceID != "wfi-dr-cancel" {
		t.Errorf("StopByProject not called as expected: %+v", fake)
	}
}

func TestDeepResearch_MaxWaitExceeded_ReportsStillRunning(t *testing.T) {
	shrinkDeepResearchTimings(t, time.Millisecond, time.Nanosecond)
	env := newConsoleTestEnv(t)
	fake := &fakeOrchestrator{startInstanceID: "wfi-dr-slow"}
	env.deps.Orch = fake
	env.seedGlobalDeepResearchInstance(t, "wfi-dr-slow", "active")

	reg, err := BuildRegistry(env.deps)
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}
	toolEnv := NewToolEnv(env.deps, "sess-1", testProjectID)

	out, isErr, err := invoke(t, reg, toolEnv, "deep_research", `{"question":"why?"}`)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if !isErr || !strings.Contains(out, "still running") {
		t.Errorf("out=%q isErr=%v, want still running message", out, isErr)
	}
}
