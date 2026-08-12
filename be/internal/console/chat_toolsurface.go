package console

import (
	"context"
	"sync"
	"time"

	"be/internal/logger"
	"be/internal/repo"
	"be/internal/ws"
)

// toolSurfaceGrace is how long a bridged engine (spawner.ConsoleEngine's
// UsesToolBridge) gets to prove its `agent mcp-external` bridge came up before
// the chat is declared tool-less. The bridge is launched by the CLI at boot and
// lists tools during MCP init, so first contact normally lands in seconds; the
// margin covers a cold CLI start on a loaded machine.
const toolSurfaceGrace = 90 * time.Second

// toolSurfaceNotice is the transcript row a chat gets when its bridge never
// made contact. It is deliberately blunt in both directions: the human needs to
// know the session cannot act before spending an hour on it, and the model
// needs to stop reporting on work it has no way to perform.
const toolSurfaceNotice = "⚠️ **This chat has no nrflo tools.** Its `agent mcp-external` bridge never " +
	"connected, so every delegation, workflow, ticket, and findings tool is absent from the conversation — " +
	"whatever the model appears to know about running work is injected context, not something it can act on. " +
	"Close and reopen the chat; if it recurs, check the server log for `console chat: tool surface never came up`."

// toolSurface tracks, process-globally, which console sessions have had at
// least one authenticated tool-route contact from their bridge. Keyed by
// session id and dropped when the chat closes. Process-global (not a
// ChatService field) because the writer is an HTTP handler in package api,
// which holds no ChatService — the same shape spawner's restart/cost stores use.
var toolSurface = struct {
	mu   sync.Mutex
	live map[string]bool
}{live: map[string]bool{}}

// MarkToolSurfaceLive records that sessionID's tool routes have been reached by
// an authenticated console bearer — proof the bridge launched, adopted the
// pre-minted session, and can call tools. Called from the console tool
// handlers; safe to call on every request.
func MarkToolSurfaceLive(sessionID string) {
	if sessionID == "" {
		return
	}
	toolSurface.mu.Lock()
	toolSurface.live[sessionID] = true
	toolSurface.mu.Unlock()
}

// ToolSurfaceLive reports whether sessionID's bridge has ever made contact.
func ToolSurfaceLive(sessionID string) bool {
	toolSurface.mu.Lock()
	defer toolSurface.mu.Unlock()
	return toolSurface.live[sessionID]
}

// dropToolSurface clears sessionID's entry so a closed chat does not leak one,
// and so a later chat reusing the id starts unproven.
func dropToolSurface(sessionID string) {
	toolSurface.mu.Lock()
	delete(toolSurface.live, sessionID)
	toolSurface.mu.Unlock()
}

// watchToolSurface waits out the grace period and then runs the check. Split
// from checkToolSurface so tests drive the decision without a timer.
func (s *ChatService) watchToolSurface(sess *chatSession, grace time.Duration) {
	timer := time.NewTimer(grace)
	defer timer.Stop()
	<-timer.C
	s.checkToolSurface(sess)
}

// checkToolSurface reports a chat whose bridge never made contact: an ERROR
// log, a transcript row the human sees in the TUI and in history, and a seed
// note the model reads on its next turn. Silent when the surface is live, when
// the session already closed, or when the engine injects its tools in-process
// (UsesToolBridge false — nothing to fail).
//
// A false alarm is impossible in the direction that matters: the flag is set by
// the tool route itself, so "live" always means a real authenticated call
// happened. The reverse — a bridged engine that genuinely never listed tools —
// is exactly the hour-long failure this exists to surface.
func (s *ChatService) checkToolSurface(sess *chatSession) {
	if ToolSurfaceLive(sess.id) {
		return
	}
	if _, ok := s.get(sess.id); !ok {
		return
	}
	if eng := sess.getEngine(); eng == nil || !eng.UsesToolBridge() {
		return
	}

	ctx := context.Background()
	logger.Error(ctx, "console chat: tool surface never came up",
		"session_id", sess.id, "engine", sess.EngineName(), "profile", sess.Profile())

	sess.appendSeedContext("SYSTEM: your nrflo MCP tools are NOT available in this session — the bridge " +
		"that serves them never connected. Do not claim, plan, or report any work you cannot do with the " +
		"tools actually in your schema; tell the owner the session is broken and stop.")

	if s.deps.Pool != nil {
		err := repo.NewAgentMessageRepo(s.deps.Pool, s.deps.Clock).InsertBatch(sess.id,
			[]repo.MessageEntry{{Content: toolSurfaceNotice, Category: "text"}})
		if err != nil {
			logger.Error(ctx, "console chat: persist tool-surface notice failed", "session_id", sess.id, "error", err)
		} else if s.deps.WSHub != nil {
			s.deps.WSHub.BroadcastSession(&ws.Event{
				Type:      ws.EventMessagesUpdated,
				ProjectID: sess.ProjectID(),
				SessionID: sess.id,
				Data:      map[string]interface{}{"session_id": sess.id},
			})
		}
	}

	pushSessionEvent(s.deps.WSHub, sess.id, sess.ProjectID(), ws.EventConsoleChatError, map[string]interface{}{
		"text":     "nrflo tool bridge never connected — this chat has no tools",
		"is_error": true,
	})
}
