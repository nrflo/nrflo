package console

import (
	"errors"
	"testing"

	"be/internal/spawner"
)

// TestChatService_Close_DropsProactiveRestartState is the leak-fix unit test:
// before the fix, ChatService.Close never called
// spawner.DropProactiveRestartState, so globalRestartStore accumulated one
// restartState per closed console session forever. With
// proactive_restart_max_per_session=1, a session that already used its one
// restart stays permanently gated (fire=false) until its state is dropped;
// Close must drop it so a decision against the (now-reused-in-principle)
// session id fires again.
func TestChatService_Close_DropsProactiveRestartState(t *testing.T) {
	t.Parallel()
	svc, pool, _, _ := newChatTestService(t)
	if err := pool.SetConfig("proactive_restart_max_per_session", "1"); err != nil {
		t.Fatalf("SetConfig proactive_restart_max_per_session: %v", err)
	}
	if err := pool.SetConfig("proactive_restart_min_interval_sec", "0"); err != nil {
		t.Fatalf("SetConfig proactive_restart_min_interval_sec: %v", err)
	}

	sid, err := svc.Create("codex", "", "", chatTestProjectID, "", "", false)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { spawner.DropProactiveRestartState(sid) })

	spawner.NoteProactiveRestart(sid, svc.deps.Clock)

	fire, _ := spawner.ProactiveRestartDecision(pool, svc.deps.Clock, sid, 300000, 250000, 0, true, false)
	if fire {
		t.Fatal("ProactiveRestartDecision before Close = true, want false (max-per-session=1 already used)")
	}

	if err := svc.Close(sid); err != nil {
		t.Fatalf("Close: %v", err)
	}

	fire, _ = spawner.ProactiveRestartDecision(pool, svc.deps.Clock, sid, 300000, 250000, 0, true, false)
	if !fire {
		t.Error("ProactiveRestartDecision after Close = false, want true (restart state must be dropped on close)")
	}
}

// TestChatService_EngineExited_DropsProactiveRestartState covers the second
// close path (engineExited, driven by the engine's Events() channel closing
// on its own — e.g. the engine dying) rather than an explicit Close call.
func TestChatService_EngineExited_DropsProactiveRestartState(t *testing.T) {
	t.Parallel()
	svc, pool, _, factory := newChatTestService(t)
	if err := pool.SetConfig("proactive_restart_max_per_session", "1"); err != nil {
		t.Fatalf("SetConfig proactive_restart_max_per_session: %v", err)
	}
	if err := pool.SetConfig("proactive_restart_min_interval_sec", "0"); err != nil {
		t.Fatalf("SetConfig proactive_restart_min_interval_sec: %v", err)
	}

	sid, err := svc.Create("codex", "", "", chatTestProjectID, "", "", false)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { spawner.DropProactiveRestartState(sid) })
	eng := factory.last()

	spawner.NoteProactiveRestart(sid, svc.deps.Clock)
	fire, _ := spawner.ProactiveRestartDecision(pool, svc.deps.Clock, sid, 300000, 250000, 0, true, false)
	if fire {
		t.Fatal("ProactiveRestartDecision before engine exit = true, want false (max-per-session=1 already used)")
	}

	// Simulate the engine dying on its own: Stop() closes Events(), which is
	// exactly what drives engineExited via pumpChatEvents in production. Here
	// we call engineExited directly — pumpChatEvents's only additional
	// behavior on channel-close is calling this same method.
	eng.Stop()
	svc.engineExited(sid)

	fire, _ = spawner.ProactiveRestartDecision(pool, svc.deps.Clock, sid, 300000, 250000, 0, true, false)
	if !fire {
		t.Error("ProactiveRestartDecision after engine exit = false, want true (restart state must be dropped)")
	}
}

// TestChatService_CloseAuthenticated_HappyPath closes a live session for the
// matching project, mirroring AttachAuthenticated's guard but delegating to
// Close instead of returning a bearer.
func TestChatService_CloseAuthenticated_HappyPath(t *testing.T) {
	t.Parallel()
	svc, _, _, _ := newChatTestService(t)

	sid, err := svc.Create("codex", "", "", chatTestProjectID, "", "", false)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := svc.CloseAuthenticated(sid, chatTestProjectID); err != nil {
		t.Fatalf("CloseAuthenticated: %v", err)
	}

	if _, ok := svc.get(sid); ok {
		t.Error("session still live after CloseAuthenticated")
	}
	// A second close must report not-found, not silently succeed.
	if err := svc.Close(sid); err != ErrChatSessionNotFound {
		t.Errorf("Close after CloseAuthenticated = %v, want ErrChatSessionNotFound", err)
	}
}

// TestChatService_CloseAuthenticated_ProjectMismatch must not close the
// session when the caller's project does not match.
func TestChatService_CloseAuthenticated_ProjectMismatch(t *testing.T) {
	t.Parallel()
	svc, _, _, _ := newChatTestService(t)

	sid, err := svc.Create("codex", "", "", chatTestProjectID, "", "", false)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := svc.CloseAuthenticated(sid, "another-project"); !errors.Is(err, ErrChatProjectMismatch) {
		t.Fatalf("CloseAuthenticated project mismatch = %v, want ErrChatProjectMismatch", err)
	}
	if _, ok := svc.get(sid); !ok {
		t.Error("session was closed despite project mismatch")
	}
}

// TestChatService_CloseAuthenticated_UnknownSession covers both an entirely
// unknown id and an id that is no longer live (already closed).
func TestChatService_CloseAuthenticated_UnknownSession(t *testing.T) {
	t.Parallel()
	svc, _, _, _ := newChatTestService(t)

	if err := svc.CloseAuthenticated("no-such-session", chatTestProjectID); err != ErrChatSessionNotFound {
		t.Errorf("CloseAuthenticated(unknown) = %v, want ErrChatSessionNotFound", err)
	}

	sid, err := svc.Create("codex", "", "", chatTestProjectID, "", "", false)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := svc.Close(sid); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := svc.CloseAuthenticated(sid, chatTestProjectID); err != ErrChatSessionNotFound {
		t.Errorf("CloseAuthenticated(already-closed) = %v, want ErrChatSessionNotFound", err)
	}
}
