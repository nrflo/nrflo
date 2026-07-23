// Package refinery folds WS-driven console-chat events into a bounded
// working-set digest via a direct one-shot Anthropic provider.Run call — no
// spawned agent_sessions row, no workflow_instance. See CLAUDE.md.
package refinery

import (
	"context"
	"sync"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/repo"
	"be/internal/service"
	"be/internal/ws"
)

// consoleStopFoldTimeout bounds Manager.Stop's synchronous final fold. It is
// deliberately tighter than the autonomous stopFoldTimeout (20s) because
// console Stop runs inline on the HTTP chat-close path and, via
// engineExited, BEFORE pumpChatEvents' terminal console_chat.turn state=idle
// push (be/internal/console/chat_events.go:45-55).
const consoleStopFoldTimeout = 10 * time.Second

// relevantEventTypes are the WS event types a sidecar folds on; every other
// broadcast is ignored before it ever reaches a per-session channel.
var relevantEventTypes = map[string]bool{
	ws.EventFindingsUpdated:        true,
	ws.EventOrchestrationCompleted: true,
	ws.EventOrchestrationFailed:    true,
	ws.EventPlanDrafted:            true,
	ws.EventPlanRevised:            true,
	ws.EventPlanApproved:           true,
	ws.EventPlanMaterialized:       true,
}

// immediateEventTypes bypass the debounce floor: a completion is worth
// folding right away rather than waiting out the window.
var immediateEventTypes = map[string]bool{
	ws.EventOrchestrationCompleted: true,
	ws.EventOrchestrationFailed:    true,
}

// Manager implements ws.Listener, routing relevant broadcasts by project id
// to per-session sidecars (Start/Stop lifecycle keyed by console-chat
// session id). Registered as a hub listener before Hub.Run (RegisterListener
// is pre-Run only), so OnEvent must never block.
type Manager struct {
	pool           *db.Pool
	clock          clock.Clock
	digestRepo     *repo.RefineryDigestRepo
	runRepo        *repo.RefineryRunRepo
	systemAgentSvc *service.SystemAgentDefinitionService
	modelSvc       *service.ModelService

	mu        sync.Mutex
	sidecars  map[string]*sidecar            // sessionID -> sidecar
	byProject map[string]map[string]*sidecar // projectID -> sessionID -> sidecar

	autonomousMu sync.Mutex
	autonomous   map[string]*autonomousSession // sessionID -> autonomous sidecar state

	slotsMu sync.Mutex
	slots   map[string]*slotLock // "workflowInstanceID/nodeID" -> refcounted per-slot lock

	// costAttributor feeds a fold's provider usage into the folded session's
	// running cost store. nil-safe: unset in tests that never call
	// SetCostAttributor, so fold cost is simply not attributed there.
	costAttributor func(sessionID string, in, out, cacheRead, cacheWrite int)

	// broadcaster emits a WS event, injected from server.go (= hub.Broadcast).
	// nil-safe: unset in tests that never call SetBroadcaster, so a fold
	// simply doesn't broadcast there.
	broadcaster func(*ws.Event)
}

// NewManager constructs a Manager over pool/clk. Shared by server wiring;
// see api/server_console_chat.go.
func NewManager(pool *db.Pool, clk clock.Clock) *Manager {
	modelSvc := service.NewModelService(pool, clk)
	return &Manager{
		pool:           pool,
		clock:          clk,
		digestRepo:     repo.NewRefineryDigestRepo(pool, clk),
		runRepo:        repo.NewRefineryRunRepo(pool, clk),
		systemAgentSvc: service.NewSystemAgentDefinitionService(pool, clk, modelSvc),
		modelSvc:       modelSvc,
		sidecars:       make(map[string]*sidecar),
		byProject:      make(map[string]map[string]*sidecar),
		autonomous:     make(map[string]*autonomousSession),
		slots:          make(map[string]*slotLock),
	}
}

// SetCostAttributor injects the running-cost feed for autonomous folds
// (= spawner.AddSessionCostUsage). Kept as a setter rather than a NewManager
// param so existing fold_test.go/manager_test.go construction call sites stay
// unchanged. nil-safe: never called from a test, cost simply isn't attributed.
func (m *Manager) SetCostAttributor(fn func(sessionID string, in, out, cacheRead, cacheWrite int)) {
	m.costAttributor = fn
}

// SetBroadcaster injects the WS emission seam for the autonomous fold path
// (= ws.Hub.Broadcast). Kept as a setter rather than a NewManager param so
// existing fold_test.go/manager_test.go construction call sites stay
// unchanged. nil-safe: never called from a test, a fold simply doesn't
// broadcast.
func (m *Manager) SetBroadcaster(fn func(*ws.Event)) {
	m.broadcaster = fn
}

// Start launches a sidecar for a console-chat session. Idempotent — a second
// Start for an already-live session id is a no-op.
func (m *Manager) Start(sessionID, projectID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.sidecars[sessionID]; ok {
		return
	}
	sc := newSidecar(sessionID, projectID, m.clock, m.fold)
	m.sidecars[sessionID] = sc
	if m.byProject[projectID] == nil {
		m.byProject[projectID] = make(map[string]*sidecar)
	}
	m.byProject[projectID][sessionID] = sc
	sc.run()
}

// Stop tears a session's sidecar down. Idempotent — unknown session id is a
// no-op.
func (m *Manager) Stop(sessionID string) {
	m.mu.Lock()
	sc, ok := m.sidecars[sessionID]
	if ok {
		delete(m.sidecars, sessionID)
		if byID := m.byProject[sc.projectID]; byID != nil {
			delete(byID, sessionID)
			if len(byID) == 0 {
				delete(m.byProject, sc.projectID)
			}
		}
	}
	m.mu.Unlock()
	if ok {
		ctx, cancel := context.WithTimeout(context.Background(), consoleStopFoldTimeout)
		sc.flush(ctx)
		cancel()
		sc.stop()
	}
}

// Flush requests a bounded synchronous final fold of sessionID's buffered
// events. Idempotent/no-op for an unknown, never-started, or refinery-off
// session, mirroring Stop's idempotence. Best-effort: a timed-out ctx simply
// leaves the digest unflushed, never an error.
func (m *Manager) Flush(ctx context.Context, sessionID string) {
	m.mu.Lock()
	sc, ok := m.sidecars[sessionID]
	m.mu.Unlock()
	if !ok {
		return
	}
	sc.flush(ctx)
}

// OnEvent implements ws.Listener. Non-blocking: it only appends a compact
// event line and signals each matching sidecar's trigger channel.
func (m *Manager) OnEvent(ev *ws.Event) {
	if relevantEventTypes[ev.Type] {
		m.mu.Lock()
		sessions := m.byProject[ev.ProjectID]
		targets := make([]*sidecar, 0, len(sessions))
		for _, sc := range sessions {
			targets = append(targets, sc)
		}
		m.mu.Unlock()
		if len(targets) > 0 {
			line := formatEventLine(ev)
			immediate := immediateEventTypes[ev.Type]
			for _, sc := range targets {
				sc.push(line, immediate)
			}
		}
	}

	// Autonomous route: a task-boundary findings.updated for a live
	// autonomous session triggers an immediate fold — no debounce, no
	// buffered event line (the fold reads the agent_messages delta itself).
	if ev.Type == ws.EventFindingsUpdated && ev.SessionID != "" {
		m.autonomousMu.Lock()
		as, ok := m.autonomous[ev.SessionID]
		m.autonomousMu.Unlock()
		if ok {
			// Non-empty sentinel line: sidecar.runFold no-ops on an empty
			// buffer, but the autonomous fold ignores buffered lines
			// entirely — it reads the agent_messages delta itself.
			as.sc.push(ev.Type, true)
		}
	}
}
