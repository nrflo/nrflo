package api

import (
	"fmt"
	"net/http"

	"be/internal/clock"
	"be/internal/config"
	"be/internal/console"
	"be/internal/db"
	ptyPkg "be/internal/pty"
	"be/internal/repo"
	"be/internal/service"
	"be/internal/spawner"
	"be/internal/ws"
)

// newConsoleChatService builds the server's console.ChatService and wires its
// engine-factory seam to s.consoleChatEngineFunc (nil = spawner.GetConsoleEngine)
// so tests can substitute a fake engine after NewServer returns — same
// injectable-seam pattern as cliAdapterFunc/specImportAdapterFunc. Split out
// of server.go to keep that file's line count under its baseline.
func newConsoleChatService(s *Server, cfg *config.Config, pool *db.Pool, clk clock.Clock, hub *ws.Hub, ptyMgr *ptyPkg.Manager, consoleHub *spawner.ConsoleHub, errorSvc *service.ErrorService) *console.ChatService {
	svc := console.NewChatService(console.ChatDeps{
		Pool:      pool,
		Clock:     clk,
		WSHub:     hub,
		PTY:       ptyMgr,
		Hub:       consoleHub,
		ErrorSvc:  errorSvc,
		ServerURL: fmt.Sprintf("http://127.0.0.1:%d", cfg.Server.Port),
	})
	svc.SetEngineFactory(func(name string, deps spawner.EngineDeps) (spawner.ConsoleEngine, error) {
		if s.consoleChatEngineFunc != nil {
			return s.consoleChatEngineFunc(name, deps)
		}
		return spawner.GetConsoleEngine(name, deps)
	})
	return svc
}

// newWSHandler builds the /api/v1/ws handler with the console-chat session
// authorizer installed, so a subscribe_session action is gated by the same
// predicate as the chat REST routes.
func (s *Server) newWSHandler() *ws.Handler {
	h := ws.NewHandler(s.wsHub)
	h.SetSessionAuthorizer(s.consoleChatSessionAuthorizer)
	return h
}

// consoleChatSessionAuthorizer builds one WS connection's SessionAuthorizer:
// session channels carry live console-chat assistant output, so a client may
// only subscribe to a console_chat session it would also be allowed to read
// over REST (same admin/service-principal/own-bearer predicate). The principal
// is snapshotted at upgrade time; the session id is resolved per subscribe, so
// a closed/unknown/non-chat session denies.
func (s *Server) consoleChatSessionAuthorizer(r *http.Request) ws.SessionAuthorizer {
	principal := consolePrincipalOf(r)
	return func(sessionID string) bool {
		sess, err := repo.NewAgentSessionRepo(s.pool, s.clock).GetConsoleChat(sessionID)
		if err != nil || sess == nil {
			return false
		}
		return authorizedForConsoleSession(principal, sess)
	}
}
