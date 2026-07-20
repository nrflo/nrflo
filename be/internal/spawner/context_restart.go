package spawner

import (
	"sync"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/ws"
)

const (
	defaultProactiveRestartThreshold      = 250000 // proactive_restart_threshold_default
	defaultProactiveRestartMinIntervalSec = 600    // proactive_restart_min_interval_sec
	defaultProactiveRestartMaxPerSession  = 0      // proactive_restart_max_per_session; 0 = unlimited but logged
	defaultProactiveBoundaryWindowTurns   = 10     // proactive_restart_boundary_window_turns
	defaultProactiveRestartConsolePct     = 75     // proactive_restart_console_pct
)

// restartState is one session's proactive-restart bookkeeping: how many
// times it has rotated, when it last did, and the most recent task-boundary
// signal observed for it (a finding recorded, at the ledger's current turn).
type restartState struct {
	restartsDone     int
	lastRestartAt    time.Time
	lastBoundaryTurn int
	lastBoundaryAt   time.Time
}

// restartStore is a process-global, session-keyed table of restartState —
// mirrors ledgerStore (ledger_store.go): one instance shared by every
// spawner and console.ChatService in the process, dropped when a session
// ends or rotates.
type restartStore struct {
	mu       sync.Mutex
	sessions map[string]*restartState
}

func newRestartStore() *restartStore {
	return &restartStore{sessions: make(map[string]*restartState)}
}

func (s *restartStore) snapshot(sessionID string) restartState {
	s.mu.Lock()
	defer s.mu.Unlock()
	if st, ok := s.sessions[sessionID]; ok {
		return *st
	}
	return restartState{}
}

func (s *restartStore) get(sessionID string) *restartState {
	st, ok := s.sessions[sessionID]
	if !ok {
		st = &restartState{}
		s.sessions[sessionID] = st
	}
	return st
}

func (s *restartStore) noteBoundary(sessionID string, turn int, now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.get(sessionID)
	st.lastBoundaryTurn = turn
	st.lastBoundaryAt = now
}

func (s *restartStore) noteRestart(sessionID string, now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.get(sessionID)
	st.restartsDone++
	st.lastRestartAt = now
}

func (s *restartStore) drop(sessionID string) {
	s.mu.Lock()
	delete(s.sessions, sessionID)
	s.mu.Unlock()
}

// globalRestartStore is the process-wide store production code writes
// through; tests construct their own newRestartStore() instead of reaching
// through this singleton.
var globalRestartStore = newRestartStore()

// resolveProactiveRestartThreshold resolves the effective per-session
// proactive-restart token ceiling: a non-nil def override wins (0 =
// disabled, >0 = ceiling), NULL (or a nil def — system agents, global
// workflows) falls through to defaultThreshold (the global
// proactive_restart_threshold_default config value). Mirrors
// resolveContextBudget (context_watcher.go).
func resolveProactiveRestartThreshold(def *model.AgentDefinition, defaultThreshold int) int {
	if def != nil && def.ProactiveRestartThresholdTokens != nil {
		return *def.ProactiveRestartThresholdTokens
	}
	return defaultThreshold
}

// ProactiveRestartThresholdDefault reads the global
// proactive_restart_threshold_default config knob.
func ProactiveRestartThresholdDefault(pool *db.Pool) int {
	return contextConfigInt(pool, "proactive_restart_threshold_default", defaultProactiveRestartThreshold)
}

// ProactiveRestartConsoleThreshold resolves the console-rotation token
// ceiling: a percentage (proactive_restart_console_pct, default 75) of the
// live context window (maxContext), capped at budget when budget>0 (a
// console.Profile's ContextBudgetTokens — e.g. t0-decider's 50k rotates a
// 200k-window claude chat at 50k, well under the 75% pct-of-window ceiling).
// Console engines track usage as a fraction of their window, not a ledger
// total, so the autonomous ledger-sized default (250k) never applies here —
// it exceeds a claude window (200k) and would make console rotation dead on
// arrival. pct<=0 (or an unknown window) returns 0, which shouldRotate treats
// as disabled — budget does not override that.
func ProactiveRestartConsoleThreshold(pool *db.Pool, maxContext, budget int) int {
	if maxContext <= 0 {
		return 0
	}
	pct := contextConfigInt(pool, "proactive_restart_console_pct", defaultProactiveRestartConsolePct)
	if pct <= 0 {
		return 0
	}
	if pct > 100 {
		pct = 100
	}
	threshold := maxContext * pct / 100
	if budget > 0 && budget < threshold {
		return budget
	}
	return threshold
}

// ProactiveRestartDecision evaluates the shared proactive-restart policy for
// sessionID: the caller supplies its own token/idle/turn/plan-in-flight
// signals, and the process-global restart store supplies the
// min-interval/max-per-session/last-boundary-turn bookkeeping that safety
// rail applies uniformly whether the caller is the autonomous monitor
// (checkProactiveRestart) or console chat rotation
// (console.ChatService.maybeRotate). Callers with no ledger-turn concept
// (console) pass currentTurn==0 — the call site itself is the boundary.
func ProactiveRestartDecision(pool *db.Pool, clk clock.Clock, sessionID string, currentTokens, thresholdTokens, currentTurn int, idle, lastPlanItemInFlight bool) (fire bool, tokensBefore int) {
	st := globalRestartStore.snapshot(sessionID)
	d := rotateDecision{
		Now:                  clk.Now(),
		CurrentTokens:        currentTokens,
		ThresholdTokens:      thresholdTokens,
		CurrentTurn:          currentTurn,
		Idle:                 idle,
		LastPlanItemInFlight: lastPlanItemInFlight,
		LastBoundaryTurn:     st.lastBoundaryTurn,
		BoundaryWindowTurns:  contextConfigInt(pool, "proactive_restart_boundary_window_turns", defaultProactiveBoundaryWindowTurns),
		LastRestartAt:        st.lastRestartAt,
		MinInterval:          time.Duration(contextConfigInt(pool, "proactive_restart_min_interval_sec", defaultProactiveRestartMinIntervalSec)) * time.Second,
		RestartsDone:         st.restartsDone,
		MaxPerSession:        contextConfigInt(pool, "proactive_restart_max_per_session", defaultProactiveRestartMaxPerSession),
	}
	return shouldRotate(d)
}

// NoteProactiveRestart records that sessionID just fired a proactive
// restart, so its NEXT decision sees the updated min-interval/max-per-session
// bookkeeping.
func NoteProactiveRestart(sessionID string, clk clock.Clock) {
	globalRestartStore.noteRestart(sessionID, clk.Now())
}

// DropProactiveRestartState removes sessionID's bookkeeping — called when a
// session ends or rotates, mirroring globalLedgerStore.drop.
func DropProactiveRestartState(sessionID string) {
	globalRestartStore.drop(sessionID)
}

// proactiveBoundaryEventTypes are the WS event types that can stamp a
// task-boundary turn — mirrors refinery.Manager's relevantEventTypes set.
// Only findings.updated currently carries a session id
// (service.BroadcastFromCtx stamps Event.SessionID from BroadcastCtx);
// orchestration/plan events are instance-scoped, not session-scoped, so
// OnEvent below cannot attribute them to one session and they are no-ops in
// practice — kept in the set for parity/future-proofing.
var proactiveBoundaryEventTypes = map[string]bool{
	ws.EventFindingsUpdated:        true,
	ws.EventOrchestrationCompleted: true,
	ws.EventOrchestrationFailed:    true,
	ws.EventPlanMaterialized:       true,
}

// ProactiveRestartCoordinator implements ws.Listener: it stamps a
// task-boundary turn onto the process-global restart store whenever a
// session's event carries a session id — the "finding recorded" task-
// boundary signal the proactive-restart policy's boundary-window gate
// checks. Registered once, pre-Run, in api/server.go (mirrors
// refinery.Manager).
type ProactiveRestartCoordinator struct {
	clock clock.Clock
}

// NewProactiveRestartCoordinator constructs a coordinator over clk.
func NewProactiveRestartCoordinator(clk clock.Clock) *ProactiveRestartCoordinator {
	return &ProactiveRestartCoordinator{clock: clk}
}

// OnEvent implements ws.Listener. Non-blocking: a single map lookup plus a
// ledger read.
func (c *ProactiveRestartCoordinator) OnEvent(ev *ws.Event) {
	if !proactiveBoundaryEventTypes[ev.Type] || ev.SessionID == "" {
		return
	}
	turn, ok := globalLedgerStore.turnNow(ev.SessionID)
	if !ok {
		// No tracked ledger for this session (api mode watcher never ran, or
		// it already dropped) — nothing to stamp a boundary turn against.
		return
	}
	globalRestartStore.noteBoundary(ev.SessionID, turn, c.clock.Now())
}
