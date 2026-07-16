package console

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/id"
	"be/internal/logger"
	"be/internal/model"
	ptyPkg "be/internal/pty"
	"be/internal/repo"
	"be/internal/service"
	"be/internal/spawner"
	"be/internal/ws"
)

// ErrChatSessionNotFound is returned when a session id does not resolve to a
// live console-chat session held by this ChatService.
var ErrChatSessionNotFound = errors.New("console_chat_session_not_found")

// ChatDeps bundles what ChatService needs to mint and drive console-chat
// engines. WSHub/PTY/Hub are the server's shared infrastructure (same
// *ws.Hub, *pty.Manager, *spawner.ConsoleHub every other console/spawner path
// uses) — a claude chat's PreToolUse approvals arrive through Hub exactly
// like a human console session's. Tools is the console tool-profile Deps
// (Pool/Clock/WSHub plus the service handles BuildRegistry/NewToolEnv need) —
// every engine gets it via EngineDeps.API unconditionally; claude/codex
// simply ignore it, the same as they ignore PTY/Hub/NrfloPath.
type ChatDeps struct {
	Pool      *db.Pool
	Clock     clock.Clock
	WSHub     *ws.Hub
	PTY       *ptyPkg.Manager
	Hub       *spawner.ConsoleHub
	ErrorSvc  *service.ErrorService
	ServerURL string
	Tools     Deps
}

// ChatService owns kind='console_chat' agent_sessions lifecycle: the
// spawner.ConsoleEngine, the turn state machine, the approval relay, and the
// engine->WS event pump. Chat rows are excluded from orchestrator/stall/
// restart machinery structurally (engines hold no processInfo,
// spawner/console_engine.go) and in SQL (the existing user-facing listings
// filter kind='workflow_agent') — never by a kind check added here.
type ChatService struct {
	deps ChatDeps

	// engineFactory is the test seam: defaults to spawner.GetConsoleEngine, the
	// ONE place an engine name is compared (Rule 6).
	engineFactory func(name string, deps spawner.EngineDeps) (spawner.ConsoleEngine, error)

	mu       sync.Mutex
	sessions map[string]*chatSession
}

// NewChatService creates a ChatService backed by deps.
func NewChatService(deps ChatDeps) *ChatService {
	return &ChatService{
		deps:          deps,
		engineFactory: spawner.GetConsoleEngine,
		sessions:      make(map[string]*chatSession),
	}
}

// SetEngineFactory overrides the engine constructor (test seam for a fake
// engine, same style as Server.cliAdapterFunc/specImportAdapterFunc).
func (s *ChatService) SetEngineFactory(f func(name string, deps spawner.EngineDeps) (spawner.ConsoleEngine, error)) {
	s.engineFactory = f
}

// Create validates the project, resolves the selected models row (if any), mints a
// kind='console_chat' agent_sessions row with a bearer spawn token, starts the
// engine, and launches its event pump. Mirrors
// service.ConsoleService.CreateSession (project Exists check, uuid id,
// id.MintToken, StartedAt, Status=user_interactive).
//
// The row is inserted BEFORE the engine starts: the engine's env already
// carries NRFLO_CONSOLE_TOKEN/NRFLO_CONSOLE_SESSION_ID (chat_spec.go) and
// NRF_SESSION_ID, so the first thing the freshly-spawned CLI does — an MCP
// tools/list through `agent mcp-external`, a SessionStart hook — authenticates
// against this row. A failed Start closes it again rather than leaving an open
// session with no engine.
func (s *ChatService) Create(engine, modelID, effort, projectID string) (sessionID string, err error) {
	sessionID, _, err = s.create(engine, modelID, effort, projectID)
	return sessionID, err
}

// CreateAuthenticated is the trusted-local variant used by the Unix socket.
// It returns the session bearer so a native TUI can drive only the chat it
// just created. HTTP callers use Create and never receive this credential.
func (s *ChatService) CreateAuthenticated(engine, modelID, effort, projectID string) (sessionID, token string, err error) {
	return s.create(engine, modelID, effort, projectID)
}

func (s *ChatService) create(engine, modelID, effort, projectID string) (sessionID, token string, err error) {
	exists, err := repo.NewProjectRepo(s.deps.Pool, s.deps.Clock).Exists(projectID)
	if err != nil {
		return "", "", fmt.Errorf("check project: %w", err)
	}
	if !exists {
		return "", "", service.ErrConsoleProjectNotFound
	}

	sessionID = uuid.New().String()
	token = id.MintToken()

	spec, err := buildChatEngineSpec(s.deps.Pool, s.deps.Clock, chatSpecParams{
		SessionID:       sessionID,
		ProjectID:       projectID,
		Engine:          engine,
		ModelID:         modelID,
		ReasoningEffort: effort,
		SpawnToken:      token,
		ServerURL:       s.deps.ServerURL,
	})
	if err != nil {
		return "", "", err
	}

	sink := &chatSink{
		pool:      s.deps.Pool,
		clock:     s.deps.Clock,
		wsHub:     s.deps.WSHub,
		errorSvc:  s.deps.ErrorSvc,
		sessionID: sessionID,
		projectID: projectID,
	}

	// Built unconditionally (no engine name-check) — a chat engine gets the
	// same tool profile the mcp-external bridge serves; claude/codex simply
	// ignore EngineDeps.API.
	reg, err := BuildRegistry(s.deps.Tools)
	if err != nil {
		return "", "", fmt.Errorf("build console tool registry: %w", err)
	}

	eng, err := s.engineFactory(engine, spawner.EngineDeps{
		Sink:      sink,
		PTY:       s.deps.PTY,
		Hub:       s.deps.Hub,
		NrfloPath: resolveNrfloPath(),
		API: spawner.APIEngineDeps{
			Pool:     s.deps.Pool,
			Clock:    s.deps.Clock,
			Tools:    Specs(reg),
			Handlers: reg,
			ToolEnv:  NewToolEnv(s.deps.Tools, sessionID, projectID),
		},
	})
	if err != nil {
		return "", "", fmt.Errorf("build console engine: %w", err)
	}

	sessionRepo := repo.NewAgentSessionRepo(s.deps.Pool, s.deps.Clock)
	now := s.deps.Clock.Now().UTC().Format(time.RFC3339Nano)
	row := &model.AgentSession{
		ID:         sessionID,
		ProjectID:  projectID,
		TicketID:   "",
		Phase:      "console_chat",
		NodeID:     "console_chat",
		AgentType:  "console_chat",
		Status:     model.AgentSessionUserInteractive,
		Kind:       model.AgentSessionKindConsoleChat,
		SpawnToken: sql.NullString{String: token, Valid: true},
		StartedAt:  sql.NullString{String: now, Valid: true},
	}
	if modelID != "" {
		row.ModelID = sql.NullString{String: modelID, Valid: true}
	}
	row.ConsoleEngine = sql.NullString{String: engine, Valid: true}
	if err := sessionRepo.Create(row); err != nil {
		return "", "", fmt.Errorf("create console_chat session: %w", err)
	}

	if err := eng.Start(context.Background(), spec); err != nil {
		eng.Stop()
		_, _ = sessionRepo.CloseConsoleChat(sessionID)
		return "", "", fmt.Errorf("start console engine: %w", err)
	}

	sess := newChatSession(sessionID, projectID, engine, modelID, spec.WorkDir, eng)
	s.mu.Lock()
	s.sessions[sessionID] = sess
	s.mu.Unlock()

	go pumpChatEvents(s.deps.Pool, s.deps.Clock, s.deps.WSHub, sess, func() { s.engineExited(sessionID) })

	return sessionID, token, nil
}

// engineExited tears the session down after its engine's event channel closed
// (Stop, or the engine dying on its own). Idempotent: a Close-initiated exit
// finds the session already gone from the map and does nothing, so the row is
// never closed twice.
func (s *ChatService) engineExited(sid string) {
	s.mu.Lock()
	_, ok := s.sessions[sid]
	if ok {
		delete(s.sessions, sid)
	}
	s.mu.Unlock()
	if !ok {
		return
	}
	if _, err := repo.NewAgentSessionRepo(s.deps.Pool, s.deps.Clock).CloseConsoleChat(sid); err != nil {
		logger.Error(context.Background(), "console chat: close row after engine exit", "session_id", sid, "error", err)
	}
}

// SendMessage submits one user turn. Returns spawner.ErrTurnActive when a
// turn is already in flight (the REST handler maps this to 409) — rejected
// locally via chatSession.beginTurn before ever reaching the engine, so the
// reject is deterministic without a round trip.
func (s *ChatService) SendMessage(sid, text string) error {
	sess, ok := s.get(sid)
	if !ok {
		return ErrChatSessionNotFound
	}
	if err := sess.beginTurn(); err != nil {
		return err
	}
	if err := sess.engine.SendUserTurn(context.Background(), text); err != nil {
		sess.endTurn()
		return err
	}
	return nil
}

// ReplyApproval forwards an already-mapped decision to the engine. The
// engine's own EventApprovalResolved (handled once, by pumpChatEvents) is
// what resolves the pending approval and pushes console_chat.approval_resolved
// — this method does not duplicate that, since the same resolution can also
// arrive via a timeout or engine stop that never goes through here. The REST
// layer maps allow|deny to the spawner.ApprovalDecision wire vocabulary; this
// method never touches that mapping itself.
func (s *ChatService) ReplyApproval(sid, approvalID string, decision spawner.ApprovalDecision) error {
	sess, ok := s.get(sid)
	if !ok {
		return ErrChatSessionNotFound
	}
	return sess.engine.ReplyApproval(approvalID, decision)
}

// Interrupt cancels the active turn without closing the chat session.
func (s *ChatService) Interrupt(ctx context.Context, sid string) error {
	sess, ok := s.get(sid)
	if !ok {
		return ErrChatSessionNotFound
	}
	return sess.engine.InterruptTurn(ctx)
}

// Close stops the engine (its Events channel closing ends the event pump)
// and closes the DB row, killing its bearer token.
func (s *ChatService) Close(sid string) error {
	s.mu.Lock()
	sess, ok := s.sessions[sid]
	if ok {
		delete(s.sessions, sid)
	}
	s.mu.Unlock()
	if !ok {
		return ErrChatSessionNotFound
	}
	sess.engine.Stop()
	if _, err := repo.NewAgentSessionRepo(s.deps.Pool, s.deps.Clock).CloseConsoleChat(sid); err != nil {
		return fmt.Errorf("close console_chat session: %w", err)
	}
	return nil
}

// get reports whether sid is a live chat session held by this service.
func (s *ChatService) get(sid string) (*chatSession, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[sid]
	return sess, ok
}
