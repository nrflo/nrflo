package console

import (
	"errors"
	"strings"
	"testing"
	"time"

	"be/internal/repo"
	"be/internal/ws"
)

// createT0DeciderChat creates a t0-decider chat via the fake engine seam and
// returns its session id.
func createT0DeciderChat(t *testing.T, svc *ChatService) string {
	t.Helper()
	sid, err := svc.Create("claude", "", "", chatTestProjectID, "", "t0-decider", false)
	if err != nil {
		t.Fatalf("Create(t0-decider): %v", err)
	}
	return sid
}

func TestChatService_SwitchModel_NonT0Decider_ErrSiblingRequiresT0Decider(t *testing.T) {
	t.Parallel()
	svc, _, _, _ := newChatTestService(t)
	sid, err := svc.Create("codex", "", "", chatTestProjectID, "", "", false)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := svc.SwitchModel(sid, "claude", "", ""); !errors.Is(err, ErrSiblingRequiresT0Decider) {
		t.Errorf("SwitchModel on non-t0-decider chat = %v, want ErrSiblingRequiresT0Decider", err)
	}
}

func TestChatService_OpenHandsSibling_NonT0Decider_ErrSiblingRequiresT0Decider(t *testing.T) {
	t.Parallel()
	svc, _, _, _ := newChatTestService(t)
	sid, err := svc.Create("codex", "", "", chatTestProjectID, "", "", false)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := svc.OpenHandsSibling(sid); !errors.Is(err, ErrSiblingRequiresT0Decider) {
		t.Errorf("OpenHandsSibling on non-t0-decider chat = %v, want ErrSiblingRequiresT0Decider", err)
	}
}

func TestChatService_SwitchModel_UnknownSession_ErrChatSessionNotFound(t *testing.T) {
	t.Parallel()
	svc, _, _, _ := newChatTestService(t)
	if _, err := svc.SwitchModel("no-such-session", "claude", "", ""); err != ErrChatSessionNotFound {
		t.Errorf("SwitchModel(unknown) = %v, want ErrChatSessionNotFound", err)
	}
}

// TestChatService_SwitchModel_CreatesSiblingAndLeavesOriginLive is the core
// invariant: a model switch on a t0-decider chat must NEVER mutate the
// origin's running engine — it opens a distinct sibling session under the
// new engine/model, and the origin's engine stays live (not Stopped).
func TestChatService_SwitchModel_CreatesSiblingAndLeavesOriginLive(t *testing.T) {
	t.Parallel()
	svc, _, hub, factory := newChatTestService(t)
	sid := createT0DeciderChat(t, svc)
	originEngine := factory.last()

	ch := subscribeChatSession(t, hub, sid)
	siblingID, err := svc.SwitchModel(sid, "claude", "sonnet-5", "")
	if err != nil {
		t.Fatalf("SwitchModel: %v", err)
	}
	if siblingID == "" || siblingID == sid {
		t.Fatalf("SwitchModel sibling id = %q, want a new distinct session id", siblingID)
	}
	if originEngine.isStopped() {
		t.Error("origin engine was stopped by SwitchModel, want it left live")
	}
	if _, ok := svc.get(sid); !ok {
		t.Error("origin session removed from the map by SwitchModel, want it to stay")
	}
	sibSess, ok := svc.get(siblingID)
	if !ok {
		t.Fatal("sibling session not found after SwitchModel")
	}
	if sibSess.Profile() != "t0-decider" {
		t.Errorf("sibling Profile() = %q, want t0-decider (SwitchModel keeps the origin's profile)", sibSess.Profile())
	}

	ev := waitForEventType(t, ch, ws.EventConsoleChatSiblingOpened, 2*time.Second)
	if ev.Data["origin_session_id"] != sid {
		t.Errorf("sibling_opened origin_session_id = %v, want %q", ev.Data["origin_session_id"], sid)
	}
	if ev.Data["sibling_session_id"] != siblingID {
		t.Errorf("sibling_opened sibling_session_id = %v, want %q", ev.Data["sibling_session_id"], siblingID)
	}
	if ev.Data["reason"] != "model_switch" {
		t.Errorf("sibling_opened reason = %v, want model_switch", ev.Data["reason"])
	}
}

// TestChatService_SwitchModel_SeedsSiblingWithOriginDigest verifies a
// pre-existing refinery digest on the origin is prepended to the sibling's
// first sent turn (consumed exactly once).
func TestChatService_SwitchModel_SeedsSiblingWithOriginDigest(t *testing.T) {
	t.Parallel()
	svc, pool, _, factory := newChatTestService(t)
	sid := createT0DeciderChat(t, svc)

	digestRepo := repo.NewRefineryDigestRepo(pool, svc.deps.Clock)
	if _, err := digestRepo.Upsert(sid, chatTestProjectID, "origin's carried-forward digest"); err != nil {
		t.Fatalf("Upsert digest: %v", err)
	}

	siblingID, err := svc.SwitchModel(sid, "claude", "sonnet-5", "")
	if err != nil {
		t.Fatalf("SwitchModel: %v", err)
	}
	if err := svc.SendMessage(siblingID, "continue the work"); err != nil {
		t.Fatalf("SendMessage on sibling: %v", err)
	}

	sibEngine := factory.last()
	if sibEngine.turnCount() != 1 {
		t.Fatalf("sibling engine turn count = %d, want 1", sibEngine.turnCount())
	}
	sentTurn := sibEngine.turns[0]
	if !strings.Contains(sentTurn, "origin's carried-forward digest") {
		t.Errorf("sibling first turn = %q, want it to contain the origin's digest", sentTurn)
	}
	if !strings.Contains(sentTurn, "continue the work") {
		t.Errorf("sibling first turn = %q, want it to still contain the caller's own text", sentTurn)
	}
}

// TestChatService_SwitchModel_NoDigestYet_SeedsEmpty verifies a chat with no
// folded digest yet opens a sibling with no seed prefix at all — an empty
// seed is a valid, non-error outcome.
func TestChatService_SwitchModel_NoDigestYet_SeedsEmpty(t *testing.T) {
	t.Parallel()
	svc, _, _, factory := newChatTestService(t)
	sid := createT0DeciderChat(t, svc)

	siblingID, err := svc.SwitchModel(sid, "claude", "sonnet-5", "")
	if err != nil {
		t.Fatalf("SwitchModel: %v", err)
	}
	if err := svc.SendMessage(siblingID, "hello"); err != nil {
		t.Fatalf("SendMessage on sibling: %v", err)
	}
	sibEngine := factory.last()
	if got := sibEngine.turns[0]; got != "hello" {
		t.Errorf("sibling first turn = %q, want exactly %q (no digest to seed)", got, "hello")
	}
}

// TestChatService_OpenHandsSibling_OpensT0HandsProfile documents a confirmed
// production bug (see be_production_bugs / chat_service_sibling.go
// OpenHandsSibling): unlike SwitchModel, which falls back to
// origin.EngineName() when its engine arg is empty, OpenHandsSibling always
// calls openSibling with engine="" and never resolves it to the t0-hands
// profile's DefaultEngine ("claude") either. Because t0-hands' DefaultModelID
// ("sonnet-5") IS a real, enabled models-registry row, buildChatEngineSpec's
// cliModelResolver rejects it: registeredEngine ("claude", derived from the
// row's anthropic provider) never equals the unresolved empty engine. Every
// call to OpenHandsSibling on today's code therefore fails — this test
// pins that (undesired) behavior so a fix is a visible, intentional test
// change rather than a silent flip. Do not "fix" this test without fixing
// OpenHandsSibling itself.
func TestChatService_OpenHandsSibling_OpensT0HandsProfile(t *testing.T) {
	t.Parallel()
	svc, pool, _, factory := newChatTestService(t)
	sid := createT0DeciderChat(t, svc)
	originEngine := factory.last()

	digestRepo := repo.NewRefineryDigestRepo(pool, svc.deps.Clock)
	if _, err := digestRepo.Upsert(sid, chatTestProjectID, "hands-seed digest"); err != nil {
		t.Fatalf("Upsert digest: %v", err)
	}

	_, err := svc.OpenHandsSibling(sid)
	if err == nil {
		t.Fatal("OpenHandsSibling: got nil error, want the known engine-resolution failure (see be_production_bugs) — if this now passes, replace this test with the intended happy-path assertions")
	}
	if !strings.Contains(err.Error(), "sonnet-5") {
		t.Errorf("OpenHandsSibling error = %v, want the model-resolution mismatch for sonnet-5", err)
	}
	if originEngine.isStopped() {
		t.Error("origin engine was stopped by a failed OpenHandsSibling, want it left live")
	}
}
