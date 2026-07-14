// Package orchestrator provides server-side workflow orchestration.
// It runs entire workflows automatically by sequencing spawner calls
// for each phase, driven from the HTTP API (web UI "Run Workflow" button).
package orchestrator

import (
	"context"
	"path/filepath"
	"sync"

	"be/internal/clock"
	ptyPkg "be/internal/pty"
	"be/internal/spawner"
	"be/internal/types"
	"be/internal/venv"
	"be/internal/ws"
)

const maxNextWorkflowOnSuccessDepth = 10
const maxCallbacks = 10 // max cumulative agent spawns via callback plans per workflow run
const reasonCancelled = "cancelled"

// RunRequest contains parameters for starting an orchestrated workflow run.
type RunRequest struct {
	ProjectID               string                   `json:"project_id"`
	TicketID                string                   `json:"ticket_id"`
	WorkflowName            string                   `json:"workflow"`
	Instructions            string                   `json:"instructions"` // User-provided instructions
	ScopeType               string                   `json:"scope_type"`   // "ticket" (default) or "project"
	Interactive             bool                     `json:"interactive"`  // If true, start with interactive PTY session before layer execution
	PlanMode                bool                     `json:"plan_mode"`    // If true, start with planning PTY session, read plan file after
	CloseTicketOnComplete   bool                     `json:"close_ticket_on_complete"`
	FinalizeSuccessCommand  string                   `json:"finalize_success_command"`
	FinalizeSuccessScriptID string                   `json:"finalize_success_script_id"`
	FinalizeFailureCommand  string                   `json:"finalize_failure_command"`
	FinalizeFailureScriptID string                   `json:"finalize_failure_script_id"`
	PauseEventCommand       string                   `json:"pause_event_command"`
	PauseEventScriptID      string                   `json:"pause_event_script_id"`
	Force                   bool                     `json:"force"`                       // If true, bypass concurrent ticket workflow guard
	EndlessLoop             bool                     `json:"endless_loop"`                // Project-scope only: auto re-run on successful completion until stopped or failed
	PlanAutoApprove         bool                     `json:"plan_auto_approve"`           // Project-scope only: self-drafting plan boundary auto-approves instead of suspending (gated by service.DynamicAutoEnabled)
	ScheduledTaskID         string                   `json:"scheduled_task_id,omitempty"` // Set by scheduler; empty for UI/API-triggered runs
	ExternalID              string                   `json:"external_id,omitempty"`       // Caller-supplied external reference ID
	ExternalContext         string                   `json:"external_context,omitempty"`  // Caller-supplied external context blob
	SeedFindings            map[string]string        `json:"seed_findings,omitempty"`     // Pre-seed findings rows at scope=workflow_instance on run start
	InputArtifacts          []types.InputArtifactRef `json:"input_artifacts,omitempty"`   // Staged uploads to attach at launch time
	LaunchDepth             int                      `json:"-"`                           // lineage distance from a human start; persisted (chain cap; bumped by run_subworkflow + next_workflow_on_success)
	ParentInstanceID        string                   `json:"-"`                           // run_subworkflow caller's instance id; persisted ("" for top-level/chain runs)
	SubworkflowDepth        int                      `json:"-"`                           // run_subworkflow nesting only; persisted (chain hops carry it unchanged)
}

// IsProjectScope returns true if this is a project-scoped run request
func (r RunRequest) IsProjectScope() bool {
	return r.ScopeType == "project"
}

// RunResult contains the result of starting an orchestrated workflow.
type RunResult struct {
	InstanceID string `json:"instance_id"`
	SessionID  string `json:"session_id,omitempty"`
	Status     string `json:"status"`
}

// worktreeInfo stores git worktree metadata for a workflow run.
type worktreeInfo struct {
	projectRoot   string // original project root (for merge commands)
	worktreePath  string
	branchName    string
	defaultBranch string
}

// runState tracks a running orchestration's cancel func and active spawners.
// spawners is a sessionID→*Spawner index maintained via spawner-side
// OnSessionRegister/OnSessionUnregister callbacks.
type runState struct {
	cancel          context.CancelFunc
	spawners        map[string]*spawner.Spawner
	done            chan struct{} // closed when runLoop goroutine exits
	callbackPlan    callbackPlan  // active callback plan; zero value = no plan
	callbackPlanIdx int           // index of the next unexecuted plan step
	failReason      string        // custom failure reason set before cancel() by FailWorkflow
}

// Orchestrator manages server-side workflow runs.
type Orchestrator struct {
	mu                sync.Mutex
	runs              map[string]*runState // wfi_id → state
	subworkflowActive int                  // in-flight sub-workflow children across all runs (global cap)
	dataPath          string
	sdkDir            string
	venvMgr           *venv.Manager
	wsHub             *ws.Hub
	clock             clock.Clock
	errorSvc          spawner.ErrorRecorder

	// OnRegisterPtyCommand is called when interactive/plan mode needs to register
	// a PTY launch for a session. The API server wires this to ptyManager.RegisterLaunch.
	OnRegisterPtyCommand func(sessionID string, l ptyPkg.Launch)

	// OnClosePtySession is called when an interactive session is killed to close and
	// remove its PTY session. The API server wires this to ptyManager.Get+Close+Remove.
	OnClosePtySession func(sessionID string)

	// PTYManager is the shared PTY session manager. Passed to spawner.Config.PTYManager
	// so the interactive CLI backend can create and manage PTY sessions directly.
	PTYManager *ptyPkg.Manager
}

// New creates a new Orchestrator.
func New(dataPath string, wsHub *ws.Hub, clk clock.Clock, errorSvc spawner.ErrorRecorder, sdkDir string) *Orchestrator {
	return &Orchestrator{
		runs:     make(map[string]*runState),
		dataPath: dataPath,
		sdkDir:   sdkDir,
		venvMgr:  venv.New(filepath.Dir(dataPath), clk),
		wsHub:    wsHub,
		clock:    clk,
		errorSvc: errorSvc,
	}
}
