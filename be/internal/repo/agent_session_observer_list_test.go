package repo

import (
	"fmt"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/model"
)

// setupObserverListDB creates a DB with two projects and returns a repo + insert helper.
func setupObserverListDB(t *testing.T) (*AgentSessionRepo, func(id, proj, kind string, status model.AgentSessionStatus, startedAt time.Time)) {
	t.Helper()
	database := newTestDB(t)
	for _, proj := range []string{"proj-a", "proj-b"} {
		if _, err := database.Exec(`INSERT INTO projects (id, name, created_at, updated_at)
			VALUES (?, 'T', datetime('now'), datetime('now'))`, proj); err != nil {
			t.Fatalf("seed project %s: %v", proj, err)
		}
	}
	r := NewAgentSessionRepo(database, clock.Real())

	insert := func(id, proj, kind string, status model.AgentSessionStatus, startedAt time.Time) {
		t.Helper()
		now := time.Now().UTC().Format(time.RFC3339Nano)
		startedAtStr := startedAt.UTC().Format(time.RFC3339Nano)
		_, err := database.Exec(`
			INSERT INTO agent_sessions
			(id, project_id, ticket_id, phase, agent_type, model_id, status, kind, observer_scope, started_at, created_at, updated_at)
			VALUES (?, ?, '', 'observer', '_observer', 'sonnet', ?, ?, 'global', ?, ?, ?)`,
			id, proj, status, kind, startedAtStr, now, now)
		if err != nil {
			t.Fatalf("insert session %s: %v", id, err)
		}
	}
	return r, insert
}

func TestListActiveObservers_Empty(t *testing.T) {
	t.Parallel()
	r, _ := setupObserverListDB(t)

	sessions, err := r.ListActiveObservers("")
	if err != nil {
		t.Fatalf("ListActiveObservers: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("sessions = %d, want 0", len(sessions))
	}
}

func TestListActiveObservers_OnlyRunningAndInteractiveReturned(t *testing.T) {
	t.Parallel()
	r, insert := setupObserverListDB(t)

	now := time.Now().UTC()
	insert("obs-running", "proj-a", "observer", model.AgentSessionRunning, now)
	insert("obs-interactive", "proj-a", "observer", model.AgentSessionUserInteractive, now.Add(-time.Minute))
	insert("obs-completed", "proj-a", "observer", model.AgentSessionCompleted, now.Add(-2*time.Minute))
	insert("obs-failed", "proj-a", "observer", model.AgentSessionFailed, now.Add(-3*time.Minute))

	sessions, err := r.ListActiveObservers("")
	if err != nil {
		t.Fatalf("ListActiveObservers: %v", err)
	}
	if len(sessions) != 2 {
		t.Errorf("sessions = %d, want 2 (running + user_interactive)", len(sessions))
	}
	for _, s := range sessions {
		if s.Status != model.AgentSessionRunning && s.Status != model.AgentSessionUserInteractive {
			t.Errorf("unexpected status %q in results", s.Status)
		}
	}
}

func TestListActiveObservers_FiltersNonObserverKind(t *testing.T) {
	t.Parallel()
	r, insert := setupObserverListDB(t)

	now := time.Now().UTC()
	insert("obs-1", "proj-a", "observer", model.AgentSessionRunning, now)
	insert("wf-1", "proj-a", "workflow_agent", model.AgentSessionRunning, now)

	sessions, err := r.ListActiveObservers("")
	if err != nil {
		t.Fatalf("ListActiveObservers: %v", err)
	}
	if len(sessions) != 1 {
		t.Errorf("sessions = %d, want 1 (only observer kind)", len(sessions))
	}
	if len(sessions) > 0 && sessions[0].ID != "obs-1" {
		t.Errorf("sessions[0].ID = %q, want obs-1", sessions[0].ID)
	}
}

func TestListActiveObservers_ProjectFilter(t *testing.T) {
	t.Parallel()
	r, insert := setupObserverListDB(t)

	now := time.Now().UTC()
	insert("obs-a", "proj-a", "observer", model.AgentSessionRunning, now)
	insert("obs-b", "proj-b", "observer", model.AgentSessionRunning, now.Add(-time.Minute))

	sessions, err := r.ListActiveObservers("proj-a")
	if err != nil {
		t.Fatalf("ListActiveObservers(proj-a): %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("sessions = %d, want 1 for proj-a", len(sessions))
	}
	if sessions[0].ID != "obs-a" {
		t.Errorf("sessions[0].ID = %q, want obs-a", sessions[0].ID)
	}
}

func TestListActiveObservers_NoProjectFilter_ReturnsAll(t *testing.T) {
	t.Parallel()
	r, insert := setupObserverListDB(t)

	now := time.Now().UTC()
	insert("obs-a", "proj-a", "observer", model.AgentSessionRunning, now)
	insert("obs-b", "proj-b", "observer", model.AgentSessionRunning, now.Add(-time.Minute))

	sessions, err := r.ListActiveObservers("")
	if err != nil {
		t.Fatalf("ListActiveObservers(\"\"): %v", err)
	}
	if len(sessions) != 2 {
		t.Errorf("sessions = %d, want 2 (all projects)", len(sessions))
	}
}

func TestListActiveObservers_OrderedByStartedAtDesc(t *testing.T) {
	t.Parallel()
	r, insert := setupObserverListDB(t)

	base := time.Now().UTC()
	for i := 0; i < 3; i++ {
		insert(fmt.Sprintf("obs-%d", i), "proj-a", "observer",
			model.AgentSessionRunning, base.Add(time.Duration(i)*time.Minute))
	}

	sessions, err := r.ListActiveObservers("")
	if err != nil {
		t.Fatalf("ListActiveObservers: %v", err)
	}
	if len(sessions) != 3 {
		t.Fatalf("sessions = %d, want 3", len(sessions))
	}
	// Newest first (DESC by started_at).
	if sessions[0].ID != "obs-2" {
		t.Errorf("sessions[0].ID = %q, want obs-2 (most recent)", sessions[0].ID)
	}
	if sessions[2].ID != "obs-0" {
		t.Errorf("sessions[2].ID = %q, want obs-0 (oldest)", sessions[2].ID)
	}
}

func TestListActiveObservers_KindFieldPopulated(t *testing.T) {
	t.Parallel()
	r, insert := setupObserverListDB(t)

	insert("obs-check", "proj-a", "observer", model.AgentSessionRunning, time.Now().UTC())

	sessions, err := r.ListActiveObservers("")
	if err != nil {
		t.Fatalf("ListActiveObservers: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("sessions = %d, want 1", len(sessions))
	}
	if sessions[0].Kind != "observer" {
		t.Errorf("Kind = %q, want observer", sessions[0].Kind)
	}
}
