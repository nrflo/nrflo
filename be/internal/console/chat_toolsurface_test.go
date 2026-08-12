package console

import (
	"strings"
	"testing"

	"be/internal/repo"
)

// sessionMessages returns sid's transcript rows.
func sessionMessages(t *testing.T, svc *ChatService, sid string) []string {
	t.Helper()
	msgs, err := repo.NewAgentMessageRepo(svc.deps.Pool, svc.deps.Clock).GetBySession(sid)
	if err != nil {
		t.Fatalf("GetBySession: %v", err)
	}
	return msgs
}

func hasToolSurfaceNotice(msgs []string) bool {
	for _, m := range msgs {
		if strings.Contains(m, "no nrflo tools") {
			return true
		}
	}
	return false
}

// A bridged engine whose mcp-external bridge never made contact gets a
// transcript row the human sees and a seed note the model reads next turn —
// the failure that previously ran silent for a whole session.
func TestChatService_CheckToolSurface_NoContact_WarnsHumanAndModel(t *testing.T) {
	t.Parallel()
	svc, _, _, _ := newChatTestService(t)
	sid, err := svc.Create("claude", "", "", chatTestProjectID, "", "", false)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	sess, ok := svc.get(sid)
	if !ok {
		t.Fatalf("session %s not held by service", sid)
	}

	svc.checkToolSurface(sess)

	if !hasToolSurfaceNotice(sessionMessages(t, svc, sid)) {
		t.Errorf("no tool-surface notice in transcript")
	}
	if seed := sess.takeSeedContext(); !strings.Contains(seed, "NOT available") {
		t.Errorf("seed context = %q, want the model told its tools are missing", seed)
	}
}

// One authenticated tool-route contact (MarkToolSurfaceLive, called by the
// api handlers) is proof the bridge came up: the watchdog stays silent.
func TestChatService_CheckToolSurface_LiveSurface_Silent(t *testing.T) {
	t.Parallel()
	svc, _, _, _ := newChatTestService(t)
	sid, err := svc.Create("claude", "", "", chatTestProjectID, "", "", false)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	sess, _ := svc.get(sid)

	MarkToolSurfaceLive(sid)
	svc.checkToolSurface(sess)

	if hasToolSurfaceNotice(sessionMessages(t, svc, sid)) {
		t.Errorf("tool-surface notice emitted for a live surface")
	}
	if seed := sess.takeSeedContext(); seed != "" {
		t.Errorf("seed context = %q, want empty", seed)
	}
}

// An engine that injects its tools in-process (api: UsesToolBridge false) has
// no bridge to fail, so it is never warned about.
func TestChatService_CheckToolSurface_NoBridgeEngine_Silent(t *testing.T) {
	t.Parallel()
	svc, _, _, factory := newChatTestService(t)
	sid, err := svc.Create("api", "", "", chatTestProjectID, "", "", false)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	eng := factory.last()
	eng.mu.Lock()
	eng.noToolBridge = true
	eng.mu.Unlock()

	sess, _ := svc.get(sid)
	svc.checkToolSurface(sess)

	if hasToolSurfaceNotice(sessionMessages(t, svc, sid)) {
		t.Errorf("tool-surface notice emitted for an in-process tool engine")
	}
}

// A chat that closed before the grace period elapsed is not warned about — the
// watchdog fires on a timer that outlives short sessions.
func TestChatService_CheckToolSurface_ClosedSession_Silent(t *testing.T) {
	t.Parallel()
	svc, _, _, _ := newChatTestService(t)
	sid, err := svc.Create("claude", "", "", chatTestProjectID, "", "", false)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	sess, _ := svc.get(sid)
	if err := svc.Close(sid); err != nil {
		t.Fatalf("Close: %v", err)
	}

	svc.checkToolSurface(sess)

	if hasToolSurfaceNotice(sessionMessages(t, svc, sid)) {
		t.Errorf("tool-surface notice emitted for a closed session")
	}
}

// Close drops the session's liveness entry, so the store cannot grow without
// bound and a later chat reusing the id starts unproven.
func TestToolSurface_DropOnClose(t *testing.T) {
	t.Parallel()
	svc, _, _, _ := newChatTestService(t)
	sid, err := svc.Create("claude", "", "", chatTestProjectID, "", "", false)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	MarkToolSurfaceLive(sid)
	if !ToolSurfaceLive(sid) {
		t.Fatalf("ToolSurfaceLive = false right after marking")
	}
	if err := svc.Close(sid); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if ToolSurfaceLive(sid) {
		t.Errorf("ToolSurfaceLive = true after Close, want the entry dropped")
	}
}
