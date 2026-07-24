package console

import (
	"testing"
	"time"

	"be/internal/repo"
	"be/internal/service"
	"be/internal/spawner"
	"be/internal/ws"
)

// TestBuildChatEngineSpec_ThreadsYoloOntoSpec verifies chatSpecParams.Yolo is
// carried through verbatim onto the resulting EngineSpec.Yolo, for both true
// and false.
func TestBuildChatEngineSpec_ThreadsYoloOntoSpec(t *testing.T) {
	t.Parallel()
	pool, clk := newSpecTestPool(t)
	seedSpecProject(t, pool, "proj-spec-yolo", "")

	for _, yolo := range []bool{true, false} {
		spec, err := buildChatEngineSpec(pool, clk, chatSpecParams{
			SessionID: "s1", ProjectID: "proj-spec-yolo", Engine: "codex", SpawnToken: "tok", Yolo: yolo,
		})
		if err != nil {
			t.Fatalf("buildChatEngineSpec(Yolo=%v): %v", yolo, err)
		}
		if spec.Yolo != yolo {
			t.Errorf("spec.Yolo = %v, want %v", spec.Yolo, yolo)
		}
	}
}

// TestConsoleYolo_DefaultOnSemantics mirrors the refinery autonomousEnabled
// idiom: absent/unset and any value other than the literal "false" reads as
// ON; only "false" reads as OFF.
func TestConsoleYolo_DefaultOnSemantics(t *testing.T) {
	t.Parallel()
	pool, clk := newSpecTestPool(t)

	if got := consoleYolo(pool, clk); !got {
		t.Errorf("consoleYolo() with unset setting = %v, want true (default-ON)", got)
	}

	settings := service.NewGlobalSettingsService(pool, clk)
	if err := settings.Set("console_yolo", "true"); err != nil {
		t.Fatalf("Set(console_yolo, true): %v", err)
	}
	if got := consoleYolo(pool, clk); !got {
		t.Errorf("consoleYolo() with console_yolo=true = %v, want true", got)
	}

	if err := settings.Set("console_yolo", "false"); err != nil {
		t.Fatalf("Set(console_yolo, false): %v", err)
	}
	if got := consoleYolo(pool, clk); got {
		t.Errorf("consoleYolo() with console_yolo=false = %v, want false", got)
	}
}

// TestChatService_Create_DefaultOnYolo_ThreadsIntoEngineSpec verifies a fresh
// chat (no console_yolo setting written) starts its engine with Yolo=true —
// the default-ON console_yolo global setting reaching EngineSpec end to end.
func TestChatService_Create_DefaultOnYolo_ThreadsIntoEngineSpec(t *testing.T) {
	t.Parallel()
	svc, _, _, factory := newChatTestService(t)

	sid, err := svc.Create("codex", "", "", chatTestProjectID, "", "", false)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	eng := factory.last()
	if eng == nil || eng.spec().SessionID != sid {
		t.Fatal("no engine started for the new session")
	}
	if !eng.spec().Yolo {
		t.Error("spec.Yolo = false on a fresh chat, want true (default-ON console_yolo)")
	}
}

// TestChatService_Create_ConsoleYoloFalse_ThreadsIntoEngineSpec verifies an
// explicit console_yolo="false" reaches EngineSpec.Yolo=false at create time.
func TestChatService_Create_ConsoleYoloFalse_ThreadsIntoEngineSpec(t *testing.T) {
	t.Parallel()
	svc, pool, _, factory := newChatTestService(t)
	if err := service.NewGlobalSettingsService(pool, svc.deps.Clock).Set("console_yolo", "false"); err != nil {
		t.Fatalf("Set(console_yolo, false): %v", err)
	}

	sid, err := svc.Create("codex", "", "", chatTestProjectID, "", "", false)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	eng := factory.last()
	if eng == nil || eng.spec().SessionID != sid {
		t.Fatal("no engine started for the new session")
	}
	if eng.spec().Yolo {
		t.Error("spec.Yolo = true with console_yolo=false, want false")
	}
}

// TestChatService_Rotation_CarriesYoloOntoNewEngine verifies a proactive
// rotation reads console_yolo fresh and threads it onto the rotated engine's
// spec, same as Create.
func TestChatService_Rotation_CarriesYoloOntoNewEngine(t *testing.T) {
	t.Parallel()
	svc, pool, hub, factory := newChatTestService(t)
	setProactiveRestartConsolePct(t, pool, "50")
	if err := service.NewGlobalSettingsService(pool, svc.deps.Clock).Set("proactive_restart_min_interval_sec", "0"); err != nil {
		t.Fatalf("set min interval: %v", err)
	}

	sid, err := svc.Create("claude", "opus-4-6", "high", chatTestProjectID, "", "", false)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	oldEng := factory.last()
	if !oldEng.spec().Yolo {
		t.Fatal("pre-rotation spec.Yolo = false, want true (default-ON)")
	}

	digestRepo := repo.NewRefineryDigestRepo(pool, svc.deps.Clock)
	if _, err := digestRepo.Upsert(sid, chatTestProjectID, "carried-forward working set"); err != nil {
		t.Fatalf("Upsert digest: %v", err)
	}
	sess, ok := svc.get(sid)
	if !ok {
		t.Fatal("session not found after Create")
	}
	sess.noteContextLeft(0) // over threshold

	ch := subscribeChatSession(t, hub, sid)
	oldEng.emit(spawner.EngineEvent{Type: spawner.EventTurnCompleted, SessionID: sid})
	waitForEventType(t, ch, ws.EventConsoleContextRotated, 2*time.Second)

	newEng := factory.last()
	if newEng == oldEng {
		t.Fatal("rotation did not construct a new engine")
	}
	if !newEng.spec().Yolo {
		t.Error("rotated spec.Yolo = false, want true (carried from console_yolo)")
	}
}
