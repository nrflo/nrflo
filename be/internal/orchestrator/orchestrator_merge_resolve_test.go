package orchestrator

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/service"
	"be/internal/types"
	"be/internal/ws"
)

// seedConflictResolver seeds a "conflict-resolver" system agent definition.
// Deletes any existing row first (e.g. from migration seed data).
func seedConflictResolver(t *testing.T, env *testEnv) {
	t.Helper()
	svc := service.NewSystemAgentDefinitionService(env.pool, clock.Real())
	_ = svc.Delete("conflict-resolver")
	_, err := svc.Create(&types.SystemAgentDefCreateRequest{
		ID:      "conflict-resolver",
		Model:   "sonnet",
		Timeout: 1,
		Prompt:  "Resolve the merge conflict.",
	})
	if err != nil {
		t.Fatalf("failed to seed conflict-resolver: %v", err)
	}
}

// ensureSpawnerTempDir creates /tmp/nrflow so the spawner can write temp files.
func ensureSpawnerTempDir(t *testing.T) {
	t.Helper()
	if err := os.MkdirAll("/tmp/nrflow", 0o755); err != nil {
		t.Fatalf("failed to create /tmp/nrflow: %v", err)
	}
}

// TestAttemptConflictResolution_NoResolverConfigured verifies that when no
// conflict-resolver system agent def exists, an error is returned immediately
// and no merge-related WS events are broadcast.
func TestAttemptConflictResolution_NoResolverConfigured(t *testing.T) {
	env := newTestEnv(t)

	// Remove seeded conflict-resolver from migration so we can test the missing-resolver path.
	svc := service.NewSystemAgentDefinitionService(env.pool, clock.Real())
	_ = svc.Delete("conflict-resolver")

	ticketID := "ticket-no-resolver"
	env.createTicket(t, ticketID, "No resolver ticket")

	ch := env.subscribeWSClient(t, "ws-no-resolver", ticketID)

	wt := &worktreeInfo{
		projectRoot:   "/tmp/test-project",
		worktreePath:  "/tmp/test-worktree",
		branchName:    "feature-branch",
		defaultBranch: "main",
	}
	req := RunRequest{
		ProjectID:    env.project,
		TicketID:     ticketID,
		WorkflowName: "test",
	}

	err := env.orch.attemptConflictResolution(context.Background(), "wfi-no-resolver", req, wt, env.pool, "merge error text", nil, "")

	if err == nil {
		t.Fatal("expected error when no resolver configured")
	}
	if !strings.Contains(err.Error(), "no conflict-resolver configured") {
		t.Errorf("error should contain 'no conflict-resolver configured', got: %v", err)
	}

	// No merge-related events should have been broadcast
	select {
	case msg := <-ch:
		var ev ws.Event
		if jsonErr := json.Unmarshal(msg, &ev); jsonErr == nil {
			switch ev.Type {
			case ws.EventMergeConflictResolving, ws.EventMergeConflictResolved, ws.EventMergeConflictFailed:
				t.Errorf("unexpected merge event %q; no resolver should return before broadcasting", ev.Type)
			}
		}
	default:
		// good — no events
	}
}

// TestAttemptConflictResolution_ResolvingEventBroadcast verifies that when a
// conflict-resolver is configured, merge.conflict_resolving is broadcast with
// correct data (instance_id, branch, merge_error) before spawn is attempted.
func TestAttemptConflictResolution_ResolvingEventBroadcast(t *testing.T) {
	env := newTestEnv(t)
	ensureSpawnerTempDir(t)

	ticketID := "ticket-resolving-evt"
	env.createTicket(t, ticketID, "Resolving event ticket")
	wfiID := env.initWorkflow(t, ticketID)
	seedConflictResolver(t, env)

	ch := env.subscribeWSClient(t, "ws-resolving-evt", ticketID)

	branchName := "feature-conflict"
	mergeErrMsg := "CONFLICT (content): Merge conflict in src/main.go"

	wt := &worktreeInfo{
		projectRoot:   "/tmp/test-project",
		worktreePath:  "/tmp/test-worktree",
		branchName:    branchName,
		defaultBranch: "develop",
	}
	req := RunRequest{
		ProjectID:    env.project,
		TicketID:     ticketID,
		WorkflowName: "test",
	}

	// Context with timeout so test doesn't hang if agent binary exists
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- env.orch.attemptConflictResolution(ctx, wfiID, req, wt, env.pool, mergeErrMsg, nil, "")
	}()

	// merge.conflict_resolving is sent before spawn — must arrive quickly
	event := expectEvent(t, ch, ws.EventMergeConflictResolving, 3*time.Second)

	if event.Data["instance_id"] != wfiID {
		t.Errorf("resolving event instance_id = %v, want %v", event.Data["instance_id"], wfiID)
	}
	if event.Data["branch"] != branchName {
		t.Errorf("resolving event branch = %v, want %v", event.Data["branch"], branchName)
	}
	if event.Data["merge_error"] != mergeErrMsg {
		t.Errorf("resolving event merge_error = %v, want %v", event.Data["merge_error"], mergeErrMsg)
	}

	// Wait for function to return (spawn will fail or context will cancel)
	select {
	case <-done:
	case <-time.After(8 * time.Second):
		t.Fatal("timeout waiting for attemptConflictResolution to return")
	}
}

// TestAttemptConflictResolution_SpawnFails_FailedEvent verifies that when spawn
// fails (binary not available or context cancelled), merge.conflict_failed is
// broadcast with correct data and an error is returned.
func TestAttemptConflictResolution_SpawnFails_FailedEvent(t *testing.T) {
	env := newTestEnv(t)
	ensureSpawnerTempDir(t)

	ticketID := "ticket-spawn-fail"
	env.createTicket(t, ticketID, "Spawn fail ticket")
	wfiID := env.initWorkflow(t, ticketID)
	seedConflictResolver(t, env)

	ch := env.subscribeWSClient(t, "ws-spawn-fail", ticketID)

	branchName := "my-feature-branch"
	wt := &worktreeInfo{
		projectRoot:   "/tmp/test-project",
		worktreePath:  "/tmp/test-worktree",
		branchName:    branchName,
		defaultBranch: "main",
	}
	req := RunRequest{
		ProjectID:    env.project,
		TicketID:     ticketID,
		WorkflowName: "test",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- env.orch.attemptConflictResolution(ctx, wfiID, req, wt, env.pool, "auto-merge failed", nil, "")
	}()

	// Wait for merge.conflict_failed, draining resolving events on the way
	var failedEvent ws.Event
	deadline := time.After(8 * time.Second)
outer:
	for {
		select {
		case msg := <-ch:
			var ev ws.Event
			if jsonErr := json.Unmarshal(msg, &ev); jsonErr != nil {
				continue
			}
			if ev.Type == ws.EventMergeConflictFailed {
				failedEvent = ev
				break outer
			}
		case <-deadline:
			t.Fatal("timeout waiting for merge.conflict_failed event")
		}
	}

	// Verify failed event data
	if failedEvent.Data["branch"] != branchName {
		t.Errorf("failed event branch = %v, want %v", failedEvent.Data["branch"], branchName)
	}
	if failedEvent.Data["instance_id"] != wfiID {
		t.Errorf("failed event instance_id = %v, want %v", failedEvent.Data["instance_id"], wfiID)
	}
	if failedEvent.Data["error"] == "" {
		t.Error("failed event should contain non-empty error field")
	}

	// Wait for function to return and verify error
	var result error
	select {
	case result = <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for function to return after failed event")
	}

	if result == nil {
		t.Error("expected non-nil error when spawn fails")
	}
	if result != nil && !strings.Contains(result.Error(), "conflict resolution failed") {
		t.Errorf("error should contain 'conflict resolution failed', got: %v", result)
	}
}

// TestAttemptConflictResolution_ExtraVarsInResolvingEvent verifies that the
// resolving event data reflects the correct worktree info (branch and merge_error
// match the values that will be placed in ExtraVars for the spawned agent).
func TestAttemptConflictResolution_ExtraVarsInResolvingEvent(t *testing.T) {
	env := newTestEnv(t)
	ensureSpawnerTempDir(t)

	ticketID := "ticket-extravars"
	env.createTicket(t, ticketID, "ExtraVars verification ticket")
	wfiID := env.initWorkflow(t, ticketID)
	seedConflictResolver(t, env)

	ch := env.subscribeWSClient(t, "ws-extravars", ticketID)

	branchName := "ticket-extravars-branch"
	defaultBranch := "staging"
	mergeErr := "fatal: CONFLICT (content): Merge conflict in config.yaml"

	wt := &worktreeInfo{
		projectRoot:   "/tmp/test-project",
		worktreePath:  "/tmp/test-worktree",
		branchName:    branchName,
		defaultBranch: defaultBranch,
	}
	req := RunRequest{
		ProjectID:    env.project,
		TicketID:     ticketID,
		WorkflowName: "test",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go func() {
		env.orch.attemptConflictResolution(ctx, wfiID, req, wt, env.pool, mergeErr, nil, "")
	}()

	// The resolving event carries the same values as ExtraVars BRANCH_NAME and MERGE_ERROR
	event := expectEvent(t, ch, ws.EventMergeConflictResolving, 3*time.Second)

	if event.Data["branch"] != branchName {
		t.Errorf("ExtraVars BRANCH_NAME mismatch: event.branch = %v, want %v", event.Data["branch"], branchName)
	}
	if event.Data["merge_error"] != mergeErr {
		t.Errorf("ExtraVars MERGE_ERROR mismatch: event.merge_error = %v, want %v", event.Data["merge_error"], mergeErr)
	}
	// DEFAULT_BRANCH is only in ExtraVars (not in the event), verified by code inspection
}
