package repo

import (
	"strings"
	"testing"

	"be/internal/model"
)

// TestSetSpawnRuntime_SetsThenPreserves verifies SetSpawnRuntime records the pid
// and final spawn_command once a backend has started, and that a later zero pid
// / empty command leaves the existing values untouched (the CASE-WHEN guards).
// This mirrors the spawn order: the row is inserted before the child starts
// (no pid yet), then filled in after Start.
func TestSetSpawnRuntime_SetsThenPreserves(t *testing.T) {
	t.Parallel()
	database, repo, wfiID := setupTestDB(t)
	defer database.Close()

	session := &model.AgentSession{
		ID:                 "spawn-rt-1",
		ProjectID:          "proj",
		TicketID:           "TKT-1",
		WorkflowInstanceID: wfiID,
		Phase:              "implementor",
		AgentType:          "implementor",
		Status:             model.AgentSessionRunning,
	}
	if err := repo.Create(session); err != nil {
		t.Fatalf("create: %v", err)
	}

	// Row created before the child started: no pid recorded yet.
	if pre, _ := repo.Get("spawn-rt-1"); pre.PID.Valid {
		t.Fatalf("pid should be NULL before SetSpawnRuntime, got %d", pre.PID.Int64)
	}

	if err := repo.SetSpawnRuntime("spawn-rt-1", 4242, "python3 /tmp/x.py"); err != nil {
		t.Fatalf("SetSpawnRuntime: %v", err)
	}
	got, _ := repo.Get("spawn-rt-1")
	if !got.PID.Valid || got.PID.Int64 != 4242 {
		t.Errorf("pid = %v/%d, want valid 4242", got.PID.Valid, got.PID.Int64)
	}
	if got.SpawnCommand.String != "python3 /tmp/x.py" {
		t.Errorf("spawn_command = %q, want %q", got.SpawnCommand.String, "python3 /tmp/x.py")
	}

	// A subsequent zero pid + empty command must not clobber the recorded values.
	if err := repo.SetSpawnRuntime("spawn-rt-1", 0, ""); err != nil {
		t.Fatalf("SetSpawnRuntime (noop): %v", err)
	}
	again, _ := repo.Get("spawn-rt-1")
	if !again.PID.Valid || again.PID.Int64 != 4242 {
		t.Errorf("pid after noop = %v/%d, want preserved 4242", again.PID.Valid, again.PID.Int64)
	}
	if again.SpawnCommand.String != "python3 /tmp/x.py" {
		t.Errorf("spawn_command after noop = %q, want preserved", again.SpawnCommand.String)
	}
}

// TestSetSpawnRuntime_MissingSession verifies a not-found id is reported rather
// than silently affecting zero rows.
func TestSetSpawnRuntime_MissingSession(t *testing.T) {
	t.Parallel()
	database, repo, _ := setupTestDB(t)
	defer database.Close()

	err := repo.SetSpawnRuntime("does-not-exist", 1, "x")
	if err == nil || !strings.Contains(err.Error(), "agent session not found") {
		t.Fatalf("want 'agent session not found' error, got %v", err)
	}
}
