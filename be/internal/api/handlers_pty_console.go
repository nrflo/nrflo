package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"

	"be/internal/console"
	"be/internal/logger"
	"be/internal/model"

	"github.com/gorilla/websocket"
)

// handleConsolePtyAttach relays a raw terminal onto a live claude console
// chat's PTY. Unlike the take-control relay (handlers_pty.go) this is a pure
// VIEWER: output arrives via the engine's ferry (the engine keeps sole
// ownership of PTY reads), input/resize go through the engine, and
// disconnecting only detaches — it never closes the PTY, never completes the
// session. Human-typed prompts persist via the UserPromptSubmit hook
// (NotifyUserPrompt returns own=false for them).
func (s *Server) handleConsolePtyAttach(w http.ResponseWriter, r *http.Request, session *model.AgentSession) {
	if !authorizedForConsoleClose(r, session) {
		writeError(w, http.StatusForbidden, "not authorized for this console chat session")
		return
	}
	if s.consoleChat == nil {
		writeError(w, http.StatusServiceUnavailable, "console chat service not available")
		return
	}

	conn, err := ptyUpgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.Error(context.Background(), "console pty ws upgrade error", "error", err)
		return
	}

	var writeMu sync.Mutex
	sink := func(data []byte) {
		writeMu.Lock()
		defer writeMu.Unlock()
		_ = conn.WriteMessage(websocket.BinaryMessage, data)
	}

	target, detach, err := s.consoleChat.AttachPTY(session.ID, sink)
	if err != nil {
		msg := "failed to attach terminal"
		switch {
		case errors.Is(err, console.ErrChatNoPTY):
			msg = "this chat's engine has no terminal (claude only)"
		case errors.Is(err, console.ErrChatSessionNotFound):
			msg = "console chat session is not live"
		}
		writeMu.Lock()
		_ = conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.ClosePolicyViolation, msg))
		writeMu.Unlock()
		conn.Close()
		return
	}
	defer detach()

	ctx := r.Context()
	logger.Info(ctx, "console pty viewer attached", "session_id", session.ID)

	for {
		msgType, data, err := conn.ReadMessage()
		if err != nil {
			break // viewer disconnected — detach only, PTY stays alive
		}
		if msgType == websocket.TextMessage {
			var msg resizeMsg
			if json.Unmarshal(data, &msg) == nil && msg.Type == "resize" {
				_ = target.ViewerResize(msg.Rows, msg.Cols)
			}
			continue
		}
		if err := target.ViewerWrite(data); err != nil {
			break
		}
	}

	conn.Close()
	logger.Info(ctx, "console pty viewer detached", "session_id", session.ID)
}
