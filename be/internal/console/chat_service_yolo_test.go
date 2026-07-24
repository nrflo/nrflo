package console

import (
	"errors"
	"testing"
	"time"

	"be/internal/repo"
	"be/internal/ws"
)

// TestChatService_SetYolo_PersistsAndPushes verifies SetYolo mutates the
// live engine, write-throughs the persisted console_yolo column, and pushes
// console_chat.yolo on the session channel with the new state.
func TestChatService_SetYolo_PersistsAndPushes(t *testing.T) {
	t.Parallel()
	svc, pool, hub, factory := newChatTestService(t)

	sid, err := svc.Create("codex", "", "", chatTestProjectID, "", "", false)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	eng := factory.last()
	if !eng.Yolo() {
		t.Fatal("pre-toggle engine Yolo() = false, want true (default-ON at create)")
	}

	ch := subscribeChatSession(t, hub, sid)
	if err := svc.SetYolo(sid, false); err != nil {
		t.Fatalf("SetYolo(sid, false): %v", err)
	}

	if eng.Yolo() {
		t.Error("engine Yolo() = true after SetYolo(sid, false), want false")
	}

	row, err := repo.NewAgentSessionRepo(pool, svc.deps.Clock).GetConsoleChat(sid)
	if err != nil {
		t.Fatalf("GetConsoleChat: %v", err)
	}
	if !row.ConsoleYolo.Valid || row.ConsoleYolo.Bool {
		t.Errorf("row.ConsoleYolo = %+v, want {true, false}", row.ConsoleYolo)
	}

	ev := waitForEventType(t, ch, ws.EventConsoleChatYolo, 2*time.Second)
	if ev.Data["yolo"] != false {
		t.Errorf("console_chat.yolo data = %v, want yolo=false", ev.Data)
	}

	// Toggle back on and verify both the engine and the push reflect it.
	if err := svc.SetYolo(sid, true); err != nil {
		t.Fatalf("SetYolo(sid, true): %v", err)
	}
	if !eng.Yolo() {
		t.Error("engine Yolo() = false after SetYolo(sid, true), want true")
	}
	ev2 := waitForEventType(t, ch, ws.EventConsoleChatYolo, 2*time.Second)
	if ev2.Data["yolo"] != true {
		t.Errorf("console_chat.yolo data = %v, want yolo=true", ev2.Data)
	}
}

// TestChatService_SetYolo_UnknownSession_ReturnsErrChatSessionNotFound
// mirrors RevokeSessionApproval's not-found contract.
func TestChatService_SetYolo_UnknownSession_ReturnsErrChatSessionNotFound(t *testing.T) {
	t.Parallel()
	svc, _, _, _ := newChatTestService(t)

	err := svc.SetYolo("no-such-session", true)
	if !errors.Is(err, ErrChatSessionNotFound) {
		t.Errorf("SetYolo(unknown) = %v, want ErrChatSessionNotFound", err)
	}
}

// TestChatService_Snapshot_ReflectsYolo verifies Snapshot reads the engine's
// live Yolo state, not a stale value.
func TestChatService_Snapshot_ReflectsYolo(t *testing.T) {
	t.Parallel()
	svc, _, _, _ := newChatTestService(t)

	sid, err := svc.Create("codex", "", "", chatTestProjectID, "", "", false)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	snap, ok := svc.Snapshot(sid)
	if !ok {
		t.Fatal("Snapshot() ok = false for a live session")
	}
	if !snap.Yolo {
		t.Error("Snapshot().Yolo = false right after create, want true (default-ON)")
	}

	if err := svc.SetYolo(sid, false); err != nil {
		t.Fatalf("SetYolo(sid, false): %v", err)
	}
	snap2, ok := svc.Snapshot(sid)
	if !ok {
		t.Fatal("Snapshot() ok = false after SetYolo")
	}
	if snap2.Yolo {
		t.Error("Snapshot().Yolo = true after SetYolo(sid, false), want false")
	}
}
