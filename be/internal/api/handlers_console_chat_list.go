package api

import (
	"database/sql"
	"net/http"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/service"
	"be/internal/spawner"
)

// consoleChatListItem is one row in GET /api/v1/console/chats.
type consoleChatListItem struct {
	SessionID   string `json:"session_id"`
	Engine      string `json:"engine"`
	Model       string `json:"model,omitempty"`
	ProjectID   string `json:"project_id"`
	Status      string `json:"status"`
	StartedAt   string `json:"started_at,omitempty"`
	EndedAt     string `json:"ended_at,omitempty"`
	ContextLeft *int   `json:"context_left,omitempty"`
	Live        bool   `json:"live"`
	Profile     string `json:"profile,omitempty"`
	Yolo        bool   `json:"yolo"`
}

// resolveYolo reads a row's persisted console_yolo column (NULL -> the
// console_yolo global default, mirroring console.consoleYolo's default-ON
// semantics) for list/detail display.
func resolveYolo(row *model.AgentSession, pool *db.Pool, clk clock.Clock) bool {
	if row.ConsoleYolo.Valid {
		return row.ConsoleYolo.Bool
	}
	val, _ := service.NewGlobalSettingsService(pool, clk).Get("console_yolo")
	return val != "false"
}

// handleListConsoleChats lists this project's kind='console_chat' sessions,
// most recently started first. `live` reflects whether ChatService still
// holds an in-memory engine for the row — a hard server kill can leave a row
// status=user_interactive with no engine, and the UI must not offer to
// resume that session.
// GET /api/v1/console/chats
func (s *Server) handleListConsoleChats(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project required")
		return
	}

	rows, err := repo.NewAgentSessionRepo(s.pool, s.clock).ListConsoleChats(projectID, 0)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	items := make([]consoleChatListItem, 0, len(rows))
	for _, row := range rows {
		item := consoleChatListItem{
			SessionID: row.ID,
			Engine:    row.ConsoleEngine.String,
			Model:     row.ModelID.String,
			ProjectID: row.ProjectID,
			Status:    string(row.Status),
			StartedAt: row.StartedAt.String,
			EndedAt:   row.EndedAt.String,
			Live:      s.consoleChat.Live(row.ID),
			Profile:   row.ConsoleProfile,
			Yolo:      resolveYolo(row, s.pool, s.clock),
		}
		if row.ContextLeft.Valid {
			v := int(row.ContextLeft.Int64)
			item.ContextLeft = &v
		}
		items = append(items, item)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"sessions": items})
}

// consoleChatApprovalItem is one pending approval in a chat detail snapshot.
// Tool + Input let a client render tool=AskUserQuestion as a question card
// (Input is the verbatim tool-input JSON with the questions array).
type consoleChatApprovalItem struct {
	ApprovalID string `json:"approval_id"`
	Kind       string `json:"kind"`
	Tool       string `json:"tool,omitempty"`
	Command    string `json:"command"`
	Cwd        string `json:"cwd"`
	Reason     string `json:"reason"`
	Input      string `json:"input,omitempty"`
}

// handleGetConsoleChat returns one console-chat session's row fields plus its
// live snapshot (turn state, work dir, pending approvals), after the {sid}
// kind guard + authz predicate (loadConsoleChatSession). This is what lets a
// reloaded page restore an in-flight approval modal and the turn spinner
// instead of losing them — a pending approval otherwise exists only as an
// ephemeral WS push. `live=false` (engine gone, e.g. a hard server kill)
// omits turn/work_dir/pending_approvals rather than fabricating them.
// GET /api/v1/console/chats/{sid}
func (s *Server) handleGetConsoleChat(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.loadConsoleChatSession(w, r)
	if !ok {
		return
	}

	resp := map[string]interface{}{
		"session_id": sess.ID,
		"engine":     sess.ConsoleEngine.String,
		"model":      sess.ModelID.String,
		"project_id": sess.ProjectID,
		"status":     string(sess.Status),
		"started_at": sess.StartedAt.String,
		"ended_at":   sess.EndedAt.String,
		"profile":    sess.ConsoleProfile,
		"yolo":       resolveYolo(sess, s.pool, s.clock),
	}
	if sess.ContextLeft.Valid {
		resp["context_left"] = int(sess.ContextLeft.Int64)
	}
	if cost, ok := spawner.SessionCost(sess.ID); ok && cost.PricingKnown {
		resp["cost_estimate"] = cost.CostUSD
	} else if v, ok := lastFlushedCostEstimate(s.pool, sess.ID); ok {
		resp["cost_estimate"] = v
	}

	snap, live := s.consoleChat.Snapshot(sess.ID)
	resp["live"] = live
	if live {
		resp["yolo"] = snap.Yolo
		approvals := make([]consoleChatApprovalItem, 0, len(snap.PendingApprovals))
		for _, a := range snap.PendingApprovals {
			approvals = append(approvals, consoleChatApprovalItem{
				ApprovalID: a.ID,
				Kind:       a.Kind,
				Tool:       a.Tool,
				Command:    a.Command,
				Cwd:        a.Cwd,
				Reason:     a.Reason,
				Input:      string(a.Raw),
			})
		}
		resp["turn"] = snap.Turn
		resp["work_dir"] = snap.WorkDir
		if snap.GitBranch != "" {
			resp["git_branch"] = snap.GitBranch
			resp["git_added"] = snap.GitAdded
			resp["git_deleted"] = snap.GitDeleted
		}
		resp["pending_approvals"] = approvals
		sessionApprovals := snap.SessionApprovals
		if sessionApprovals == nil {
			sessionApprovals = []string{}
		}
		resp["session_approvals"] = sessionApprovals
		queued := snap.QueuedPrompts
		if queued == nil {
			queued = []string{}
		}
		resp["queued_prompts"] = queued
		liveItems := make([]map[string]string, 0, len(snap.LiveItems))
		for _, item := range snap.LiveItems {
			liveItems = append(liveItems, map[string]string{"item_id": item.ID, "text": item.Text})
		}
		resp["live_items"] = liveItems
		if snap.Thinking.Text != "" {
			resp["thinking"] = map[string]string{"item_id": snap.Thinking.ID, "text": snap.Thinking.Text}
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

// lastFlushedCostEstimate reads agent_sessions.cost_estimate directly — that
// column is deliberately not part of sessionCols/scanSession (avoids the
// 30-column ripple), so a non-live session's last debounced flush is read
// with a raw query instead.
func lastFlushedCostEstimate(pool *db.Pool, sessionID string) (float64, bool) {
	var v sql.NullFloat64
	if err := pool.QueryRow(`SELECT cost_estimate FROM agent_sessions WHERE id = ?`, sessionID).Scan(&v); err != nil {
		return 0, false
	}
	return v.Float64, v.Valid
}
