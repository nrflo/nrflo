package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"be/internal/logger"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/ws"

	"github.com/gorilla/websocket"
)

// ptyUpgrader is a separate upgrader for PTY WebSocket connections.
var ptyUpgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// resizeMsg is the JSON payload for terminal resize commands.
type resizeMsg struct {
	Type string `json:"type"`
	Rows uint16 `json:"rows"`
	Cols uint16 `json:"cols"`
}

// handlePtyWebSocket upgrades to WebSocket and relays I/O to a PTY running
// `claude --resume <session_id>` in interactive mode.
// GET /api/v1/pty/{session_id}
func (s *Server) handlePtyWebSocket(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("session_id")
	if sessionID == "" {
		writeError(w, http.StatusBadRequest, "session_id required")
		return
	}

	// Look up session.
	asRepo := s.agentSessionRepo()
	session, err := asRepo.Get(sessionID)
	if err != nil {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}

	if session.Status != model.AgentSessionUserInteractive {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("session status is %s, expected user_interactive", session.Status))
		return
	}

	// Look up workflow instance for the workflow name (needed for broadcast).
	wfiRepo := repo.NewWorkflowInstanceRepo(s.pool, s.clock)
	wfi, err := wfiRepo.Get(session.WorkflowInstanceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to look up workflow instance")
		return
	}
	workflowName := wfi.WorkflowID

	// Look up project root for working directory.
	projectRepo := s.projectRepo()
	project, err := projectRepo.Get(session.ProjectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to look up project")
		return
	}

	workDir := ""
	if project.RootPath.Valid {
		workDir = project.RootPath.String
	}

	env := buildPtyEnv(session, project)

	// Get or create PTY session.
	ptySess := s.ptyManager.Get(sessionID)
	if ptySess == nil {
		ptySess, err = s.ptyManager.Create(sessionID, workDir, env)
		if err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to start pty: %v", err))
			return
		}
	}

	// Upgrade to WebSocket.
	conn, err := ptyUpgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.Error(context.Background(), "pty ws upgrade error", "error", err)
		return
	}

	ctx := r.Context()
	logger.Info(ctx, "pty ws connected", "session_id", sessionID)

	// Relay I/O between WebSocket and PTY.
	done := make(chan struct{})

	// PTY → WebSocket (read loop)
	go func() {
		defer close(done)
		buf := make([]byte, 4096)
		for {
			n, err := ptySess.Read(buf)
			if err != nil {
				return
			}
			if err := conn.WriteMessage(websocket.BinaryMessage, buf[:n]); err != nil {
				return
			}
		}
	}()

	// WebSocket → PTY (write loop)
	go func() {
		for {
			msgType, data, err := conn.ReadMessage()
			if err != nil {
				// Browser disconnected — kill PTY.
				ptySess.Close()
				return
			}

			// Text messages are JSON control commands (e.g., resize).
			if msgType == websocket.TextMessage {
				var msg resizeMsg
				if json.Unmarshal(data, &msg) == nil && msg.Type == "resize" {
					_ = ptySess.Resize(msg.Rows, msg.Cols)
				}
				continue
			}

			// Binary messages are raw terminal input.
			if _, err := ptySess.Write(data); err != nil {
				return
			}
		}
	}()

	// Wait for PTY to exit.
	select {
	case <-done:
	case <-ptySess.Done():
	}

	exitCode := ptySess.ExitCode()
	if exitCode != 0 {
		logger.Warn(ctx, "pty process exited with error", "session_id", sessionID, "exit_code", exitCode)
	}

	// PTY process exited — trigger exit-interactive flow.
	s.completePtyInteractive(session, workflowName)

	conn.WriteMessage(websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, "process exited"))
	conn.Close()
	logger.Info(ctx, "pty ws closed", "session_id", sessionID)
}

// completePtyInteractive updates the session to interactive_completed and
// unblocks the spawner, then broadcasts agent.completed.
func (s *Server) completePtyInteractive(session *model.AgentSession, workflowName string) {
	if s.orchestrator == nil {
		return
	}
	if err := s.orchestrator.CompleteInteractive(session.ID); err != nil {
		logger.Error(context.Background(), "pty complete interactive failed", "session_id", session.ID, "error", err)
		return
	}

	// Broadcast agent.completed event so UI updates.
	event := ws.NewEvent(ws.EventAgentCompleted, session.ProjectID, session.TicketID, workflowName, map[string]interface{}{
		"session_id": session.ID,
		"phase":      session.Phase,
		"agent_type": session.AgentType,
		"result":     "pass",
	})
	s.wsHub.Broadcast(event)
}

// buildPtyEnv constructs the environment for the PTY process.
// Uses the full server env (matching the normal spawner) to avoid missing vars
// that Claude needs to start, plus nrworkflow-specific vars and TERM override.
func buildPtyEnv(session *model.AgentSession, project *model.Project) []string {
	// Start with full server env, filtering out CLAUDECODE
	env := filterEnv(os.Environ(), "CLAUDECODE")

	// Ensure TERM is set for PTY
	env = setEnv(env, "TERM", "xterm-256color")

	// Set nrworkflow-specific vars (use setEnv to avoid duplicates).
	env = setEnv(env, "NRWORKFLOW_PROJECT", session.ProjectID)
	env = setEnv(env, "NRWF_WORKFLOW_INSTANCE_ID", session.WorkflowInstanceID)
	env = setEnv(env, "NRWF_SESSION_ID", session.ID)

	return env
}

// filterEnv returns a copy of env with the named variable removed.
func filterEnv(env []string, name string) []string {
	prefix := name + "="
	out := make([]string, 0, len(env))
	for _, e := range env {
		if !strings.HasPrefix(e, prefix) {
			out = append(out, e)
		}
	}
	return out
}

// setEnv sets or replaces a variable in the env slice.
func setEnv(env []string, key, value string) []string {
	prefix := key + "="
	for i, e := range env {
		if strings.HasPrefix(e, prefix) {
			env[i] = prefix + value
			return env
		}
	}
	return append(env, prefix+value)
}
