// Package stepengine is the server-owned stepwise step engine: cursor
// snapshot, evidence validation, transactional advance, and rotate decision
// for prompt_mode='stepwise' agent definitions.
//
// Import hygiene: this package depends only on db/repo/model/clock/logger/
// handoff — never on service or spawner — so P4's builtin (tools_builtin)
// and P5's spawner wiring can both import it without a cycle.
package stepengine

import (
	"context"
	"encoding/json"
	"errors"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"
)

// Sentinel errors. Reserved for infrastructure/programmer failures — an
// agent's rejected evidence or a stale/mismatched advance call is never a Go
// error, it is an Outcome with Kind==OutcomeRejected (see advance.go).
var (
	// ErrNoCursor means no agent_step_cursors row exists for (instanceID, nodeID).
	ErrNoCursor = errors.New("stepengine: no cursor for node")
	// ErrNotStepwise means the agent definition is not prompt_mode='stepwise'
	// (nil, empty mode, or nil/empty Steps).
	ErrNotStepwise = errors.New("stepengine: agent definition is not stepwise")
	// ErrBadSnapshot means the definition's Steps JSON failed to decode or was empty.
	ErrBadSnapshot = errors.New("stepengine: malformed steps snapshot")
)

// CheckRunner executes a step's `checks` commands. Mirrors
// spawner.Spawner.runValidationCommands' return shape verbatim
// (failedIdx=-1 means all passed) so P5 can adapt the real executor with no
// reshaping. A nil CheckRunner skips checks entirely (Advance treats an
// empty/nil runner as "no checks to run").
type CheckRunner interface {
	RunChecks(ctx context.Context, cmds []string) (failedIdx, exitCode int, outputTail string, err error)
}

// OutcomeKind classifies the result of Advance.
type OutcomeKind int

// OutcomeKind values.
const (
	OutcomeNext OutcomeKind = iota
	OutcomeDone
	OutcomeRotate
	OutcomeRejected
)

// Rejection carries the agent-facing reason an Advance call did not move the
// cursor forward.
type Rejection struct {
	Reason  string // "missing_evidence" | "invalid_evidence" | "check_failed" | "stale_revision" | "step_mismatch"
	Message string
}

// Outcome is the result of Advance: which state the cursor is in (or was
// rejected from), and — for OutcomeNext/OutcomeRotate — the step now current.
type Outcome struct {
	Kind         OutcomeKind
	NextStep     *model.StepDefinition
	Revision     int
	CurrentIndex int
	Rejection    *Rejection
	Replayed     bool
}

// State is the read-time decoded view of an AgentStepCursor for callers
// (P3's prompt renderer, P6's UI read model) that need the live steps +
// progress rather than the raw JSON columns.
type State struct {
	WorkflowInstanceID string
	NodeID             string
	Steps              []model.StepDefinition
	Revision           int
	CurrentIndex       int
	Completed          []model.CompletedStep
}

// Engine holds the repos + clock + injectable check runner the stepwise
// flows share.
type Engine struct {
	pool        *db.Pool
	clock       clock.Clock
	checks      CheckRunner
	cursorRepo  *repo.AgentStepCursorRepo
	findingRepo *repo.FindingRepo
	wfiRepo     *repo.WorkflowInstanceRepo
	projectRepo *repo.ProjectRepo
}

// New creates an Engine. checks may be nil — Advance then skips running any
// step's `checks` commands (P5 wires the real executor).
func New(pool *db.Pool, clk clock.Clock, checks CheckRunner) *Engine {
	return &Engine{
		pool:        pool,
		clock:       clk,
		checks:      checks,
		cursorRepo:  repo.NewAgentStepCursorRepo(pool, clk),
		findingRepo: repo.NewFindingRepo(pool, clk),
		wfiRepo:     repo.NewWorkflowInstanceRepo(pool, clk),
		projectRepo: repo.NewProjectRepo(pool, clk),
	}
}

// State reads and decodes the live cursor for (instanceID, nodeID).
// Returns ErrNoCursor when no cursor row exists.
func (e *Engine) State(instanceID, nodeID string) (*State, error) {
	c, err := e.cursorRepo.Get(instanceID, nodeID)
	if err != nil {
		return nil, ErrNoCursor
	}
	steps, err := decodeSteps([]byte(c.StepsSnapshot))
	if err != nil {
		return nil, ErrBadSnapshot
	}
	var completed []model.CompletedStep
	if err := json.Unmarshal([]byte(c.Completed), &completed); err != nil {
		return nil, ErrBadSnapshot
	}
	return &State{
		WorkflowInstanceID: c.WorkflowInstanceID,
		NodeID:             c.NodeID,
		Steps:              steps,
		Revision:           c.Revision,
		CurrentIndex:       c.CurrentIndex,
		Completed:          completed,
	}, nil
}
