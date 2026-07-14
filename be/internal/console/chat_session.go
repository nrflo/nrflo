package console

import (
	"sync"

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
	id         string
	projectID  string
	engineName string
	modelID    string
	workDir    string
	engine     spawner.ConsoleEngine

	mu      sync.Mutex
	turn    turnState
	pending map[string]bool // pending approval ids
}

func newChatSession(id, projectID, engineName, modelID, workDir string, engine spawner.ConsoleEngine) *chatSession {
	return &chatSession{
		id:         id,
		projectID:  projectID,
		engineName: engineName,
		modelID:    modelID,
		workDir:    workDir,
		engine:     engine,
		turn:       turnIdle,
		pending:    make(map[string]bool),
	}
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
	return nil
}

// endTurn transitions back to idle. Safe to call even if no turn was active
// (e.g. SendUserTurn failed after beginTurn already flipped the state).
func (c *chatSession) endTurn() {
	c.mu.Lock()
	c.turn = turnIdle
	c.mu.Unlock()
}

// addPendingApproval registers id as awaiting a decision.
func (c *chatSession) addPendingApproval(id string) {
	c.mu.Lock()
	c.pending[id] = true
	c.mu.Unlock()
}

// resolvePendingApproval removes id and reports whether it was pending.
func (c *chatSession) resolvePendingApproval(id string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.pending[id] {
		return false
	}
	delete(c.pending, id)
	return true
}
