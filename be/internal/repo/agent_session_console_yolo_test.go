package repo

import (
	"testing"

	"be/internal/clock"
	"be/internal/model"
)

func TestCreate_PersistsConsoleYolo_NullByDefault(t *testing.T) {
	t.Parallel()
	database := newTestDB(t)
	mustExecLog(t, database, `INSERT INTO projects (id, name, created_at, updated_at) VALUES ('proj-cy', 'P', datetime('now'), datetime('now'))`)

	r := NewAgentSessionRepo(database, clock.Real())
	sess := &model.AgentSession{
		ID:        "sess-cy-null",
		ProjectID: "proj-cy",
		Phase:     "p",
		AgentType: "a",
		Status:    model.AgentSessionRunning,
		Kind:      "console_chat",
	}
	if err := r.Create(sess); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := r.Get("sess-cy-null")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ConsoleYolo.Valid {
		t.Errorf("ConsoleYolo.Valid = true for a row created without an explicit value, want false (NULL)")
	}
}

func TestCreate_PersistsConsoleYolo_Explicit(t *testing.T) {
	t.Parallel()
	for _, yolo := range []bool{true, false} {
		database := newTestDB(t)
		mustExecLog(t, database, `INSERT INTO projects (id, name, created_at, updated_at) VALUES ('proj-cy2', 'P', datetime('now'), datetime('now'))`)

		r := NewAgentSessionRepo(database, clock.Real())
		sess := &model.AgentSession{
			ID:        "sess-cy-explicit",
			ProjectID: "proj-cy2",
			Phase:     "p",
			AgentType: "a",
			Status:    model.AgentSessionRunning,
			Kind:      "console_chat",
		}
		sess.ConsoleYolo.Bool = yolo
		sess.ConsoleYolo.Valid = true

		if err := r.Create(sess); err != nil {
			t.Fatalf("Create(yolo=%v): %v", yolo, err)
		}

		got, err := r.Get("sess-cy-explicit")
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if !got.ConsoleYolo.Valid || got.ConsoleYolo.Bool != yolo {
			t.Errorf("ConsoleYolo = %+v, want {true, %v}", got.ConsoleYolo, yolo)
		}
	}
}

func TestSetConsoleYolo_WriteThrough(t *testing.T) {
	t.Parallel()
	database := newTestDB(t)
	mustExecLog(t, database, `INSERT INTO projects (id, name, created_at, updated_at) VALUES ('proj-cy3', 'P', datetime('now'), datetime('now'))`)

	r := NewAgentSessionRepo(database, clock.Real())
	sess := &model.AgentSession{
		ID:        "sess-cy-set",
		ProjectID: "proj-cy3",
		Phase:     "p",
		AgentType: "a",
		Status:    model.AgentSessionRunning,
		Kind:      "console_chat",
	}
	if err := r.Create(sess); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := r.SetConsoleYolo("sess-cy-set", false); err != nil {
		t.Fatalf("SetConsoleYolo(false): %v", err)
	}
	got, err := r.Get("sess-cy-set")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !got.ConsoleYolo.Valid || got.ConsoleYolo.Bool {
		t.Errorf("ConsoleYolo after SetConsoleYolo(false) = %+v, want {true, false}", got.ConsoleYolo)
	}

	if err := r.SetConsoleYolo("sess-cy-set", true); err != nil {
		t.Fatalf("SetConsoleYolo(true): %v", err)
	}
	got, err = r.Get("sess-cy-set")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !got.ConsoleYolo.Valid || !got.ConsoleYolo.Bool {
		t.Errorf("ConsoleYolo after SetConsoleYolo(true) = %+v, want {true, true}", got.ConsoleYolo)
	}
}

func TestSetConsoleYolo_NotFound(t *testing.T) {
	t.Parallel()
	database := newTestDB(t)
	r := NewAgentSessionRepo(database, clock.Real())

	if err := r.SetConsoleYolo("does-not-exist", true); err == nil {
		t.Error("SetConsoleYolo(missing) returned nil, want error")
	}
}
