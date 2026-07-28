package console

import (
	"strings"
	"testing"

	"be/internal/repo"
)

// TestChatService_SwitchModel_FlushesOriginBeforeSeeding is the ordering
// assertion: onFlush simulates a digest landing mid-flush, and the sibling's
// first sent turn must contain it — which only happens if openSibling's
// Flush call runs BEFORE its originDigest read.
func TestChatService_SwitchModel_FlushesOriginBeforeSeeding(t *testing.T) {
	t.Parallel()
	mgr := &fakeRefineryLifecycle{}
	svc, pool, _, factory := newChatTestServiceWithRefinery(t, mgr)
	sid := createT0DeciderChat(t, svc)

	mgr.onFlush = func(sessionID string) {
		if _, err := repo.NewRefineryDigestRepo(pool, svc.deps.Clock).Upsert(sessionID, chatTestProjectID, "flushed-in digest"); err != nil {
			t.Errorf("Upsert in onFlush: %v", err)
		}
	}

	siblingID, err := svc.SwitchModel(sid, "claude", "sonnet-5", "")
	if err != nil {
		t.Fatalf("SwitchModel: %v", err)
	}
	if _, err := svc.SendMessage(siblingID, "continue the work"); err != nil {
		t.Fatalf("SendMessage on sibling: %v", err)
	}

	sibEngine := factory.last()
	if len(sibEngine.turns) == 0 {
		t.Fatal("sibling engine got no turns")
	}
	if !strings.Contains(sibEngine.turns[0], "flushed-in digest") {
		t.Errorf("sibling first turn = %q, want it to contain the flushed-in digest", sibEngine.turns[0])
	}
}

// TestChatService_OpenHandsSibling_FlushesOrigin verifies openSibling flushes
// the ORIGIN session id, never the newly created sibling's.
func TestChatService_OpenHandsSibling_FlushesOrigin(t *testing.T) {
	t.Parallel()
	mgr := &fakeRefineryLifecycle{}
	svc, _, _, _ := newChatTestServiceWithRefinery(t, mgr)
	sid := createT0DeciderChat(t, svc)

	siblingID, err := svc.OpenHandsSibling(sid)
	if err != nil {
		t.Fatalf("OpenHandsSibling: %v", err)
	}
	if got := mgr.flushCount(); got != 1 {
		t.Fatalf("flushCount after OpenHandsSibling = %d, want 1", got)
	}
	mgr.mu.Lock()
	flushed := append([]string(nil), mgr.flushed...)
	mgr.mu.Unlock()
	if len(flushed) != 1 || flushed[0] != sid {
		t.Errorf("flushed ids = %v, want [%q] (the origin, not sibling %q)", flushed, sid, siblingID)
	}
}

// TestChatService_SwitchModel_NilRefineryMgr_IsANoop is the sibling-flow
// twin of TestChatService_NilRefineryMgr_IsANoop: a nil RefineryLifecycle
// must not stop SwitchModel from opening a sibling.
func TestChatService_SwitchModel_NilRefineryMgr_IsANoop(t *testing.T) {
	t.Parallel()
	svc, _, _, _ := newChatTestServiceWithRefinery(t, nil)
	sid := createT0DeciderChat(t, svc)

	siblingID, err := svc.SwitchModel(sid, "claude", "sonnet-5", "")
	if err != nil {
		t.Fatalf("SwitchModel with nil RefineryMgr: %v", err)
	}
	if siblingID == "" || siblingID == sid {
		t.Fatalf("SwitchModel sibling id = %q, want a new distinct session id", siblingID)
	}
}
