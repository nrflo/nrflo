package console

import (
	"sync"
	"unicode/utf8"

	"be/internal/spawner"
)

// turnState is a chatSession's local view of whether a turn is in flight.
type turnState string

const (
	turnIdle    turnState = "idle"
	turnRunning turnState = "running"
)

// chatSession is the live, in-memory handle for one console-chat session:
// the engine, static identity, and the turn/pending-approval state machine.
// SendUserTurn (via beginTurn) is the only mutator of turn state; a second
// message while a turn is in flight is rejected locally (deterministic,
// without an engine round-trip) — the engine independently returns
// spawner.ErrTurnActive from the same race, which the service maps the
// same way.
type chatSession struct {
	id               string
	projectID        string
	engineName       string
	modelID          string
	reasoningEffort  string
	systemTemplateID string
	workDir          string

	mu             sync.Mutex
	engine         spawner.ConsoleEngine
	turn           turnState
	pending        map[string]*spawner.ApprovalRequest // pending approvals, keyed by approval id
	live           map[string]string
	liveOrder      []string
	thinkingID     string
	thinking       string
	maxContext     int
	contextLeftPct int
	hasContextInfo bool
}

const (
	maxLiveItems     = 64
	maxLiveItemBytes = 128 * 1024
)

func newChatSession(id, projectID, engineName, modelID, reasoningEffort, systemTemplateID, workDir string, maxContext int, engine spawner.ConsoleEngine) *chatSession {
	return &chatSession{
		id:               id,
		projectID:        projectID,
		engineName:       engineName,
		modelID:          modelID,
		reasoningEffort:  reasoningEffort,
		systemTemplateID: systemTemplateID,
		workDir:          workDir,
		engine:           engine,
		turn:             turnIdle,
		pending:          make(map[string]*spawner.ApprovalRequest),
		live:             make(map[string]string),
		maxContext:       maxContext,
	}
}

// Identity fields are immutable after construction — no lock needed.
func (c *chatSession) EngineName() string       { return c.engineName }
func (c *chatSession) ModelID() string          { return c.modelID }
func (c *chatSession) WorkDir() string          { return c.workDir }
func (c *chatSession) ProjectID() string        { return c.projectID }
func (c *chatSession) ReasoningEffort() string  { return c.reasoningEffort }
func (c *chatSession) SystemTemplateID() string { return c.systemTemplateID }
func (c *chatSession) MaxContext() int          { return c.maxContext }

// getEngine/setEngine guard sess.engine with mu — a proactive-restart
// rotation (chat_service_rotate.go) swaps it from the event-pump goroutine
// while HTTP-handler goroutines (SendMessage, approvals, PTY attach) read it
// concurrently.
func (c *chatSession) getEngine() spawner.ConsoleEngine {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.engine
}

func (c *chatSession) setEngine(e spawner.ConsoleEngine) {
	c.mu.Lock()
	c.engine = e
	c.mu.Unlock()
}

// noteContextLeft records the latest EventTokenUsage context-left percentage.
func (c *chatSession) noteContextLeft(pct int) {
	c.mu.Lock()
	c.contextLeftPct = pct
	c.hasContextInfo = true
	c.mu.Unlock()
}

// resetContext marks a freshly-rotated engine at 100% context left (0
// tokens used) until its first EventTokenUsage arrives.
func (c *chatSession) resetContext() {
	c.mu.Lock()
	c.contextLeftPct = 100
	c.hasContextInfo = true
	c.mu.Unlock()
}

// currentTokens estimates tokens-in-use from the last known context-left
// percentage and the engine's max context window — the console token source
// (claude/codex console engines have no context ledger). ok is false until
// the first EventTokenUsage (or a reset) has been observed, or the engine
// has no known max context.
func (c *chatSession) currentTokens() (tokens int, ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.hasContextInfo || c.maxContext <= 0 {
		return 0, false
	}
	pct := c.contextLeftPct
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	return c.maxContext * (100 - pct) / 100, true
}

// beginTurn transitions idle->running, or returns spawner.ErrTurnActive when
// a turn is already in flight.
func (c *chatSession) beginTurn() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.turn == turnRunning {
		return spawner.ErrTurnActive
	}
	c.turn = turnRunning
	c.clearLiveLocked()
	return nil
}

// endTurn transitions back to idle. Safe to call even if no turn was active
// (e.g. SendUserTurn failed after beginTurn already flipped the state).
func (c *chatSession) endTurn() {
	c.mu.Lock()
	c.turn = turnIdle
	c.mu.Unlock()
}

// addPendingApproval registers req as awaiting a decision.
func (c *chatSession) addPendingApproval(req *spawner.ApprovalRequest) {
	c.mu.Lock()
	c.pending[req.ID] = req
	c.mu.Unlock()
}

// resolvePendingApproval removes id and reports whether it was pending.
func (c *chatSession) resolvePendingApproval(id string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.pending[id]; !ok {
		return false
	}
	delete(c.pending, id)
	return true
}

func (c *chatSession) appendLive(id, text string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if id == "" {
		id = "assistant"
	}
	if _, exists := c.live[id]; !exists {
		c.liveOrder = append(c.liveOrder, id)
		if len(c.liveOrder) > maxLiveItems {
			delete(c.live, c.liveOrder[0])
			c.liveOrder = c.liveOrder[1:]
		}
	}
	c.live[id] = trimLive(c.live[id] + text)
}

func (c *chatSession) appendThinking(id, text string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if id != c.thinkingID {
		c.thinkingID, c.thinking = id, ""
	}
	c.thinking = trimLive(c.thinking + text)
}

func trimLive(value string) string {
	if len(value) <= maxLiveItemBytes {
		return value
	}
	value = value[len(value)-maxLiveItemBytes:]
	for len(value) > 0 && !utf8.RuneStart(value[0]) {
		value = value[1:]
	}
	return value
}

func (c *chatSession) clearLive() {
	c.mu.Lock()
	c.clearLiveLocked()
	c.mu.Unlock()
}

func (c *chatSession) clearLiveLocked() {
	c.live = make(map[string]string)
	c.liveOrder = nil
	c.thinkingID, c.thinking = "", ""
}

// snapshot returns the current turn state and every pending approval under
// lock — used by GET /console/chats/{sid} to rehydrate an in-flight approval
// card after a page reload, since a pending approval otherwise exists only as
// an ephemeral WS push.
func (c *chatSession) snapshot() chatStateSnapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	pending := make([]*spawner.ApprovalRequest, 0, len(c.pending))
	for _, req := range c.pending {
		pending = append(pending, req)
	}
	live := make([]ChatLiveItem, 0, len(c.liveOrder))
	for _, id := range c.liveOrder {
		live = append(live, ChatLiveItem{ID: id, Text: c.live[id]})
	}
	return chatStateSnapshot{
		Turn: c.turn, Pending: pending, Live: live,
		Thinking: ChatLiveItem{ID: c.thinkingID, Text: c.thinking},
	}
}

type chatStateSnapshot struct {
	Turn     turnState
	Pending  []*spawner.ApprovalRequest
	Live     []ChatLiveItem
	Thinking ChatLiveItem
}
