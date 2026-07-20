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

func TestChatService_SwitchModel_NoProfile_ErrSiblingUnsupportedProfile(t *testing.T) {
	t.Parallel()
	svc, _, _, _ := newChatTestService(t)
	sid, err := svc.Create("codex", "", "", chatTestProjectID, "", "", false)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := svc.SwitchModel(sid, "claude", "", ""); !errors.Is(err, ErrSiblingUnsupportedProfile) {
		t.Errorf("SwitchModel on no-profile chat = %v, want ErrSiblingUnsupportedProfile", err)
	}
}

func TestChatService_OpenHandsSibling_NoProfile_ErrSiblingUnsupportedProfile(t *testing.T) {
	t.Parallel()
	svc, _, _, _ := newChatTestService(t)
	sid, err := svc.Create("codex", "", "", chatTestProjectID, "", "", false)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := svc.OpenHandsSibling(sid); !errors.Is(err, ErrSiblingUnsupportedProfile) {
		t.Errorf("OpenHandsSibling on no-profile chat = %v, want ErrSiblingUnsupportedProfile", err)
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

// TestChatService_OpenHandsSibling_OpensT0HandsProfile is the happy-path
// replacement (per its own prior comment) for the pinned engine-default bug:
// create()'s `if engine == "" { engine = profile.DefaultEngine }` fix means
// OpenHandsSibling now resolves t0-hands' DefaultEngine ("claude") for its
// DefaultModelID ("sonnet-5") and opens successfully.
func TestChatService_OpenHandsSibling_OpensT0HandsProfile(t *testing.T) {
	t.Parallel()
	svc, pool, hub, factory := newChatTestService(t)
	sid := createT0DeciderChat(t, svc)
	originEngine := factory.last()

	digestRepo := repo.NewRefineryDigestRepo(pool, svc.deps.Clock)
	if _, err := digestRepo.Upsert(sid, chatTestProjectID, "hands-seed digest"); err != nil {
		t.Fatalf("Upsert digest: %v", err)
	}

	ch := subscribeChatSession(t, hub, sid)
	siblingID, err := svc.OpenHandsSibling(sid)
	if err != nil {
		t.Fatalf("OpenHandsSibling: %v", err)
	}
	if siblingID == "" || siblingID == sid {
		t.Fatalf("OpenHandsSibling sibling id = %q, want a new distinct session id", siblingID)
	}
	sibSess, ok := svc.get(siblingID)
	if !ok {
		t.Fatal("sibling session not found after OpenHandsSibling")
	}
	if sibSess.Profile() != "t0-hands" {
		t.Errorf("sibling Profile() = %q, want t0-hands", sibSess.Profile())
	}

	ev := waitForEventType(t, ch, ws.EventConsoleChatSiblingOpened, 2*time.Second)
	if ev.Data["reason"] != "hands_sibling" {
		t.Errorf("sibling_opened reason = %v, want hands_sibling", ev.Data["reason"])
	}

	if err := svc.SendMessage(siblingID, "go"); err != nil {
		t.Fatalf("SendMessage on sibling: %v", err)
	}
	sibEngine := factory.last()
	if !strings.Contains(sibEngine.turns[0], "hands-seed digest") {
		t.Errorf("sibling first turn = %q, want it to contain the seeded digest", sibEngine.turns[0])
	}

	if originEngine.isStopped() {
		t.Error("origin engine was stopped by OpenHandsSibling, want it left live")
	}
	if _, ok := svc.get(sid); !ok {
		t.Error("origin session removed from the map by OpenHandsSibling, want it to stay")
	}
}

// TestChatService_SwitchModel_T0BareOrigin_KeepsProfile verifies a t0-bare
// origin chat can SwitchModel and the sibling stays under t0-bare.
func TestChatService_SwitchModel_T0BareOrigin_KeepsProfile(t *testing.T) {
	t.Parallel()
	svc, _, _, _ := newChatTestService(t)
	sid, err := svc.Create("claude", "", "", chatTestProjectID, "", "t0-bare", false)
	if err != nil {
		t.Fatalf("Create(t0-bare): %v", err)
	}

	siblingID, err := svc.SwitchModel(sid, "claude", "sonnet-5", "")
	if err != nil {
		t.Fatalf("SwitchModel: %v", err)
	}
	sibSess, ok := svc.get(siblingID)
	if !ok {
		t.Fatal("sibling session not found after SwitchModel")
	}
	if sibSess.Profile() != "t0-bare" {
		t.Errorf("sibling Profile() = %q, want t0-bare (SwitchModel keeps the origin's profile)", sibSess.Profile())
	}
}

// TestChatService_OpenHandsSibling_T0BareOrigin_OpensT0Hands verifies a
// t0-bare origin (not just t0-decider) can open a t0-hands sibling — the
// gate is Profile.SiblingFlows, not a hardcoded profile name.
func TestChatService_OpenHandsSibling_T0BareOrigin_OpensT0Hands(t *testing.T) {
	t.Parallel()
	svc, _, _, _ := newChatTestService(t)
	sid, err := svc.Create("claude", "", "", chatTestProjectID, "", "t0-bare", false)
	if err != nil {
		t.Fatalf("Create(t0-bare): %v", err)
	}

	siblingID, err := svc.OpenHandsSibling(sid)
	if err != nil {
		t.Fatalf("OpenHandsSibling: %v", err)
	}
	sibSess, ok := svc.get(siblingID)
	if !ok {
		t.Fatal("sibling session not found after OpenHandsSibling")
	}
	if sibSess.Profile() != "t0-hands" {
		t.Errorf("sibling Profile() = %q, want t0-hands", sibSess.Profile())
	}
}

// TestChatService_SwitchModel_T0HandsOrigin_NowAllowed verifies a t0-hands
// origin — previously refused under the hardcoded t0-decider-only gate —
// can now SwitchModel/OpenHandsSibling per Profile.SiblingFlows.
func TestChatService_SwitchModel_T0HandsOrigin_NowAllowed(t *testing.T) {
	t.Parallel()
	svc, _, _, _ := newChatTestService(t)
	sid, err := svc.Create("claude", "", "", chatTestProjectID, "", "t0-hands", false)
	if err != nil {
		t.Fatalf("Create(t0-hands): %v", err)
	}

	if _, err := svc.SwitchModel(sid, "claude", "sonnet-5", ""); err != nil {
		t.Errorf("SwitchModel on t0-hands origin: %v, want nil (SiblingFlows allows it)", err)
	}
	if _, err := svc.OpenHandsSibling(sid); err != nil {
		t.Errorf("OpenHandsSibling on t0-hands origin: %v, want nil (SiblingFlows allows it)", err)
	}
}
