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
	// RefineryMgr starts/stops the per-session refinery sidecar. Nil-safe —
	// tests and any server wiring that never sets it get today's behavior
	// (no digest folding). See chat_service_refinery.go.
	RefineryMgr RefineryLifecycle
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
func (s *ChatService) Create(engine, modelID, effort, projectID, systemTemplateID, profileName string, refineryEnabled bool) (sessionID string, err error) {
	sessionID, _, err = s.create(engine, modelID, effort, projectID, systemTemplateID, profileName, refineryEnabled)
	return sessionID, err
}

// CreateAuthenticated is the trusted-local variant used by the Unix socket.
// It returns the session bearer so a native TUI can drive only the chat it
// just created. HTTP callers use Create and never receive this credential.
func (s *ChatService) CreateAuthenticated(engine, modelID, effort, projectID, systemTemplateID, profileName string, refineryEnabled bool) (sessionID, token string, err error) {
	return s.create(engine, modelID, effort, projectID, systemTemplateID, profileName, refineryEnabled)
}

func (s *ChatService) create(engine, modelID, effort, projectID, systemTemplateID, profileName string, refineryEnabled bool) (sessionID, token string, err error) {
	exists, err := repo.NewProjectRepo(s.deps.Pool, s.deps.Clock).Exists(projectID)
	if err != nil {
		return "", "", fmt.Errorf("check project: %w", err)
	}
	if !exists {
		return "", "", service.ErrConsoleProjectNotFound
	}
	profile, err := ProfileByName(profileName)
	if err != nil {
		return "", "", err
	}

	sessionID = uuid.New().String()
	token = id.MintToken()

	// systemTemplateID and effort are per-create overrides; the profile's own
	// system_template_id/default_effort apply only when the caller left them
	// empty (t0-hands ships a blank SystemTemplateID/DefaultEffort, so this is
	// a no-op for it).
	if systemTemplateID == "" {
		systemTemplateID = profile.SystemTemplateID
	}
	spec, err := buildChatEngineSpec(s.deps.Pool, s.deps.Clock, chatSpecParams{
		SessionID:           sessionID,
		ProjectID:           projectID,
		Engine:              engine,
		ModelID:             modelID,
		ReasoningEffort:     effort,
		SpawnToken:          token,
		ServerURL:           s.deps.ServerURL,
		SystemTemplateID:    systemTemplateID,
		NativeToolPolicy:    profile.NativeToolPolicy,
		ContextBudgetTokens: profile.ContextBudgetTokens,
		DefaultModelID:      profile.DefaultModelID,
		DefaultEffort:       profile.DefaultEffort,
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
	// ignore EngineDeps.API. profile.Catalogue (nil for no profile) restricts
	// it to the profile's allowlist.
	reg, err := BuildRegistry(s.deps.Tools, profile.Catalogue)
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

	// effectiveModelID is what actually started (profile default when the
	// caller left modelID empty) — persisted on the row and used for cost
	// registration so a profile-default chat (e.g. t0-decider with no
	// explicit modelID) still gets priced.
	effectiveModelID := modelID
	if effectiveModelID == "" {
		effectiveModelID = profile.DefaultModelID
	}

	sessionRepo := repo.NewAgentSessionRepo(s.deps.Pool, s.deps.Clock)
	now := s.deps.Clock.Now().UTC().Format(time.RFC3339Nano)
	row := &model.AgentSession{
		ID:             sessionID,
		ProjectID:      projectID,
		TicketID:       "",
		Phase:          "console_chat",
		NodeID:         "console_chat",
		AgentType:      "console_chat",
		Status:         model.AgentSessionUserInteractive,
		Kind:           model.AgentSessionKindConsoleChat,
		SpawnToken:     sql.NullString{String: token, Valid: true},
		StartedAt:      sql.NullString{String: now, Valid: true},
		ConsoleProfile: profileName,
	}
	if effectiveModelID != "" {
		row.ModelID = sql.NullString{String: effectiveModelID, Valid: true}
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

	// effectiveModelID is the registry slug the caller supplied (or the
	// profile default) — the same id chatModelResolver looked pricing up
	// under, so it resolves the identical models row here.
	spawner.RegisterSessionCost(sessionID, effectiveModelID, s.deps.Pool, s.deps.Clock, func(snap spawner.CostSnapshot) {
		pushSessionEvent(s.deps.WSHub, sessionID, projectID, ws.EventSessionCostUpdated, map[string]interface{}{
			"session_id":    sessionID,
			"cost_estimate": snap.CostUSD,
			"pricing_known": snap.PricingKnown,
		})
	})

	if s.deps.RefineryMgr != nil && s.refineryEffective(refineryEnabled, profile.RefineryDefault) {
		s.deps.RefineryMgr.Start(sessionID, projectID)
	}

	sess := newChatSession(sessionID, projectID, engine, effectiveModelID, effort, systemTemplateID, spec.WorkDir, profileName, spec.MaxContext, eng)
	s.mu.Lock()
	s.sessions[sessionID] = sess
	s.mu.Unlock()

	go pumpChatEvents(s.deps.Pool, s.deps.Clock, s.deps.WSHub, sess, func() { s.engineExited(sessionID) }, s.maybeRotate)

	return sessionID, token, nil
}

// get reports whether sid is a live chat session held by this service.
func (s *ChatService) get(sid string) (*chatSession, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[sid]
	return sess, ok
}
