package integration

import (
	"testing"
	"time"

	"be/internal/model"
	"be/internal/repo"
	"be/internal/service"
	"be/internal/types"
)

// chain_timestamps_test.go: tests for started_at/completed_at timestamp columns
// added to chain_executions in migration 000045.

// createTimestampTestChain creates a single-ticket chain for timestamp tests.
// Uses env.Clock for deterministic time control.
func createTimestampTestChain(t *testing.T, env *TestEnv, name string) *model.ChainExecution {
	t.Helper()
	createChainTickets(t, env, map[string]time.Time{"ts1": env.Clock.Now()})
	chainSvc := service.NewChainService(env.Pool, env.Clock)
	chain, err := chainSvc.CreateChain(env.ProjectID, &types.ChainCreateRequest{
		Name:         name,
		WorkflowName: "test",
		TicketIDs:    []string{"ts1"},
	})
	if err != nil {
		t.Fatalf("CreateChain(%q) failed: %v", name, err)
	}
	return chain
}

// TestChainTimestamps_CreateHasNullTimestamps verifies a new chain has nil StartedAt
// and CompletedAt — migration adds nullable columns defaulting to NULL.
func TestChainTimestamps_CreateHasNullTimestamps(t *testing.T) {
	env := NewTestEnv(t)
	chain := createTimestampTestChain(t, env, "null ts")

	chainRepo := repo.NewChainRepo(env.Pool, env.Clock)
	got, err := chainRepo.Get(chain.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.StartedAt != nil {
		t.Errorf("StartedAt = %v, want nil", got.StartedAt)
	}
	if got.CompletedAt != nil {
		t.Errorf("CompletedAt = %v, want nil", got.CompletedAt)
	}
}

// TestChainTimestamps_UpdateStatusRunning_SetsStartedAt verifies started_at is set
// when transitioning to running, and CompletedAt remains nil.
func TestChainTimestamps_UpdateStatusRunning_SetsStartedAt(t *testing.T) {
	env := NewTestEnv(t)
	chain := createTimestampTestChain(t, env, "running ts")

	env.Clock.Advance(time.Second)
	expectedStart := env.Clock.Now().UTC()

	chainRepo := repo.NewChainRepo(env.Pool, env.Clock)
	if err := chainRepo.UpdateStatus(chain.ID, model.ChainStatusRunning); err != nil {
		t.Fatalf("UpdateStatus(running): %v", err)
	}

	got, err := chainRepo.Get(chain.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.StartedAt == nil {
		t.Fatal("StartedAt = nil after running transition")
	}
	if !got.StartedAt.Equal(expectedStart) {
		t.Errorf("StartedAt = %v, want %v", got.StartedAt.UTC(), expectedStart)
	}
	if got.CompletedAt != nil {
		t.Errorf("CompletedAt = %v, want nil", got.CompletedAt)
	}
	if got.Status != model.ChainStatusRunning {
		t.Errorf("Status = %v, want running", got.Status)
	}
}

// TestChainTimestamps_UpdateStatusRunning_COALESCE verifies started_at is NOT
// overwritten on a second running transition (COALESCE idempotency).
func TestChainTimestamps_UpdateStatusRunning_COALESCE(t *testing.T) {
	env := NewTestEnv(t)
	chain := createTimestampTestChain(t, env, "coalesce ts")

	env.Clock.Advance(time.Second)
	chainRepo := repo.NewChainRepo(env.Pool, env.Clock)
	if err := chainRepo.UpdateStatus(chain.ID, model.ChainStatusRunning); err != nil {
		t.Fatalf("first UpdateStatus(running): %v", err)
	}

	first, err := chainRepo.Get(chain.ID)
	if err != nil {
		t.Fatalf("Get after first running: %v", err)
	}
	if first.StartedAt == nil {
		t.Fatal("StartedAt nil after first running")
	}
	originalStart := *first.StartedAt

	env.Clock.Advance(time.Second) // advance so a new write would differ
	if err := chainRepo.UpdateStatus(chain.ID, model.ChainStatusRunning); err != nil {
		t.Fatalf("second UpdateStatus(running): %v", err)
	}

	second, err := chainRepo.Get(chain.ID)
	if err != nil {
		t.Fatalf("Get after second running: %v", err)
	}
	if second.StartedAt == nil {
		t.Fatal("StartedAt nil after second running")
	}
	if !second.StartedAt.Equal(originalStart) {
		t.Errorf("COALESCE broken: StartedAt changed from %v to %v",
			originalStart.UTC(), second.StartedAt.UTC())
	}
}

// TestChainTimestamps_TerminalStatuses_SetsCompletedAt verifies completed_at is set
// for completed, failed, and canceled status transitions.
func TestChainTimestamps_TerminalStatuses_SetsCompletedAt(t *testing.T) {
	cases := []model.ChainStatus{
		model.ChainStatusCompleted,
		model.ChainStatusFailed,
		model.ChainStatusCanceled,
	}

	for _, tc := range cases {
		tc := tc
		t.Run(string(tc), func(t *testing.T) {
			env := NewTestEnv(t)
			chain := createTimestampTestChain(t, env, "terminal-"+string(tc))
			chainRepo := repo.NewChainRepo(env.Pool, env.Clock)

			env.Clock.Advance(time.Second)
			if err := chainRepo.UpdateStatus(chain.ID, model.ChainStatusRunning); err != nil {
				t.Fatalf("UpdateStatus(running): %v", err)
			}

			env.Clock.Advance(time.Second)
			expectedEnd := env.Clock.Now().UTC()
			if err := chainRepo.UpdateStatus(chain.ID, tc); err != nil {
				t.Fatalf("UpdateStatus(%s): %v", tc, err)
			}

			got, err := chainRepo.Get(chain.ID)
			if err != nil {
				t.Fatalf("Get: %v", err)
			}
			if got.CompletedAt == nil {
				t.Fatalf("CompletedAt nil after %s", tc)
			}
			if !got.CompletedAt.Equal(expectedEnd) {
				t.Errorf("CompletedAt = %v, want %v", got.CompletedAt.UTC(), expectedEnd)
			}
			if got.StartedAt == nil {
				t.Error("StartedAt nil — should be preserved from running transition")
			}
			if got.Status != tc {
				t.Errorf("Status = %v, want %v", got.Status, tc)
			}
		})
	}
}

// TestChainTimestamps_List_ReturnsTimestamps verifies List (which uses scanChainWithCounts)
// returns started_at and completed_at correctly alongside count columns.
func TestChainTimestamps_List_ReturnsTimestamps(t *testing.T) {
	env := NewTestEnv(t)
	chain := createTimestampTestChain(t, env, "list ts")
	chainRepo := repo.NewChainRepo(env.Pool, env.Clock)

	env.Clock.Advance(time.Second)
	expectedStart := env.Clock.Now().UTC()
	if err := chainRepo.UpdateStatus(chain.ID, model.ChainStatusRunning); err != nil {
		t.Fatalf("UpdateStatus(running): %v", err)
	}

	env.Clock.Advance(time.Second)
	expectedEnd := env.Clock.Now().UTC()
	if err := chainRepo.UpdateStatus(chain.ID, model.ChainStatusCompleted); err != nil {
		t.Fatalf("UpdateStatus(completed): %v", err)
	}

	chains, err := chainRepo.List(env.ProjectID, "", "")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(chains) != 1 {
		t.Fatalf("List returned %d chains, want 1", len(chains))
	}
	got := chains[0]
	if got.StartedAt == nil {
		t.Fatal("StartedAt nil in List result")
	}
	if got.CompletedAt == nil {
		t.Fatal("CompletedAt nil in List result")
	}
	if !got.StartedAt.Equal(expectedStart) {
		t.Errorf("StartedAt = %v, want %v", got.StartedAt.UTC(), expectedStart)
	}
	if !got.CompletedAt.Equal(expectedEnd) {
		t.Errorf("CompletedAt = %v, want %v", got.CompletedAt.UTC(), expectedEnd)
	}
}

// TestChainTimestamps_List_PendingChainHasNullTimestamps verifies a pending chain
// (never transitioned to running) returns nil timestamps via List.
func TestChainTimestamps_List_PendingChainHasNullTimestamps(t *testing.T) {
	env := NewTestEnv(t)
	createTimestampTestChain(t, env, "pending null")

	chainRepo := repo.NewChainRepo(env.Pool, env.Clock)
	chains, err := chainRepo.List(env.ProjectID, "", "")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(chains) == 0 {
		t.Fatal("List returned no chains")
	}
	got := chains[0]
	if got.StartedAt != nil {
		t.Errorf("StartedAt = %v, want nil for pending chain", got.StartedAt)
	}
	if got.CompletedAt != nil {
		t.Errorf("CompletedAt = %v, want nil for pending chain", got.CompletedAt)
	}
}

// TestChainTimestamps_FullLifecycle covers the complete timestamp lifecycle:
// Create (nil/nil) → Running (started_at set) → Running again (idempotent) → Completed (both set, start < end).
func TestChainTimestamps_FullLifecycle(t *testing.T) {
	env := NewTestEnv(t)
	chain := createTimestampTestChain(t, env, "lifecycle")
	chainRepo := repo.NewChainRepo(env.Pool, env.Clock)

	// Step 1: nil timestamps after create
	created, err := chainRepo.Get(chain.ID)
	if err != nil {
		t.Fatalf("step1 Get: %v", err)
	}
	if created.StartedAt != nil || created.CompletedAt != nil {
		t.Errorf("step1: want nil/nil, got started=%v completed=%v",
			created.StartedAt, created.CompletedAt)
	}

	// Step 2: running → started_at set
	env.Clock.Advance(time.Second)
	startedAtExpected := env.Clock.Now().UTC()
	if err := chainRepo.UpdateStatus(chain.ID, model.ChainStatusRunning); err != nil {
		t.Fatalf("step2 UpdateStatus(running): %v", err)
	}
	afterRunning, err := chainRepo.Get(chain.ID)
	if err != nil {
		t.Fatalf("step2 Get: %v", err)
	}
	if afterRunning.StartedAt == nil {
		t.Fatal("step2: StartedAt nil")
	}
	if !afterRunning.StartedAt.Equal(startedAtExpected) {
		t.Errorf("step2: StartedAt = %v, want %v", afterRunning.StartedAt.UTC(), startedAtExpected)
	}
	startedAt := *afterRunning.StartedAt

	// Step 3: running again → started_at unchanged (COALESCE)
	env.Clock.Advance(time.Second)
	if err := chainRepo.UpdateStatus(chain.ID, model.ChainStatusRunning); err != nil {
		t.Fatalf("step3 UpdateStatus(running): %v", err)
	}
	afterRunning2, err := chainRepo.Get(chain.ID)
	if err != nil {
		t.Fatalf("step3 Get: %v", err)
	}
	if afterRunning2.StartedAt == nil || !afterRunning2.StartedAt.Equal(startedAt) {
		t.Errorf("step3: StartedAt changed: got %v, want %v",
			afterRunning2.StartedAt, startedAt)
	}

	// Step 4: completed → completed_at set, started_at preserved, start < end
	env.Clock.Advance(time.Second)
	completedAtExpected := env.Clock.Now().UTC()
	if err := chainRepo.UpdateStatus(chain.ID, model.ChainStatusCompleted); err != nil {
		t.Fatalf("step4 UpdateStatus(completed): %v", err)
	}
	done, err := chainRepo.Get(chain.ID)
	if err != nil {
		t.Fatalf("step4 Get: %v", err)
	}
	if done.StartedAt == nil || !done.StartedAt.Equal(startedAt) {
		t.Errorf("step4: StartedAt changed: got %v, want %v", done.StartedAt, startedAt)
	}
	if done.CompletedAt == nil {
		t.Fatal("step4: CompletedAt nil")
	}
	if !done.CompletedAt.Equal(completedAtExpected) {
		t.Errorf("step4: CompletedAt = %v, want %v", done.CompletedAt.UTC(), completedAtExpected)
	}
	if !done.CompletedAt.After(startedAt) {
		t.Errorf("step4: CompletedAt (%v) not after StartedAt (%v)",
			done.CompletedAt, startedAt)
	}
	if done.Status != model.ChainStatusCompleted {
		t.Errorf("step4: Status = %v, want completed", done.Status)
	}
}
