package apirun

import (
	"context"
	"encoding/json"
	"fmt"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/service"
	"be/internal/spawner/apirun/provider"
	"be/internal/types"
)

// ToolHandler is the unified contract every API-mode tool implements. The
// runner looks up handlers by Spec().Name when dispatching tool_use blocks.
type ToolHandler interface {
	Spec() provider.ToolSpec
	Invoke(ctx context.Context, env ToolEnv, input json.RawMessage) (output string, isError bool, err error)
}

// MediaToolHandler is an optional extension a tool implements when its result
// includes image/document content blocks (e.g. read_document feeding a scanned
// PDF to the model for native OCR). The runner prefers InvokeMedia when the
// handler satisfies this interface; otherwise it falls back to Invoke.
type MediaToolHandler interface {
	ToolHandler
	InvokeMedia(ctx context.Context, env ToolEnv, input json.RawMessage) (output string, media []provider.MediaBlock, isError bool, err error)
}

// WorkflowController allows API-mode agents to continue or fail a workflow instance.
// Nil-safe; guard with env.WorkflowControl == nil before calling.
type WorkflowController interface {
	ContinueWorkflow(ctx context.Context, projectID, instanceID, instructions string) error
	FailWorkflow(ctx context.Context, projectID, instanceID, reason string) error
}

// ConsultantSpawner allows API-mode agents to ask a named consultant a question inline.
// Nil-safe; guard with env.Consultant == nil before calling.
type ConsultantSpawner interface {
	Consult(ctx context.Context, callerSessionID, consultantID, question string) (string, error)
}

// DelegateRequest is the delegate builtin's parsed input, threaded through to
// the Delegator implementation unchanged. The bounded sync wait (wait_sec) is
// owned by the delegate/get_delegation builtin handlers around the two
// non-blocking Delegator calls, so it is not carried here.
type DelegateRequest struct {
	Tier      string   // "extractor" (_t2_extractor) or "executor" (_t1_executor)
	Brief     string   // required; templated per fanout item
	Context   string   // inline context, capped at 4KB by the handler
	Artifacts []string // artifact names materialized for the worker(s)
	Fanout    []string // one worker per item; empty = a single worker
}

// Delegator lets API-mode agents spawn tier-resolved delegate workers
// (single or fanout) downward and poll async delegations, mirroring
// ConsultantSpawner/SubworkflowRunner's async-with-poll shape. Nil-safe;
// guard with env.Delegator == nil. Neither method blocks — the delegate/
// get_delegation builtin handlers own the bounded, heartbeated wait (mirrors
// run_subworkflow's pollSubworkflow around SubworkflowRunner.GetSubworkflow).
type Delegator interface {
	// Delegate spawns one detached worker per fanout item (or a single worker
	// when Fanout is empty) under the caller's context and returns
	// immediately: {"delegation_id":...,"status":"running"}.
	Delegate(ctx context.Context, callerSessionID string, req DelegateRequest) (string, error)
	// GetDelegation returns the delegation's current aggregated status
	// without blocking: worker findings for finished workers, "running" while
	// any are still in flight.
	GetDelegation(ctx context.Context, callerSessionID, delegationID string) (string, error)
	// MergeDelegation merges an isolated delegation's server-committed
	// branch into the live checkout's current branch, server-side — the
	// sanctioned path for landing executor results without any agent
	// running git against the live tree.
	MergeDelegation(ctx context.Context, callerSessionID, delegationID string) (string, error)
}

// StepSession is the spawner-side seam the complete_step builtin uses to
// read rotation signals and drive a stepwise agent's rotation, mirroring
// ConsultantSpawner/Delegator's shape. Nil means no rotation policy (tests,
// console) — the builtin treats a nil env.Steps as "never rotate".
type StepSession interface {
	// RotateSignals returns the calling session's current context-usage
	// tokens and its resolved rotate threshold (0,0 when unknown/disabled).
	RotateSignals(sessionID string) (contextTokens, thresholdTokens int)
	// NoteStepBoundary stamps a task-boundary signal for sessionID at the
	// current ledger turn, the same signal a finding-recorded boundary gives
	// the idle proactive-restart watcher.
	NoteStepBoundary(sessionID string)
	// RequestStepRotation asks the spawner to kill and relaunch sessionID as
	// a rotation (not a failure/continuation) — non-blocking.
	RequestStepRotation(sessionID string)
	// RunStepChecks executes a step's `checks` commands for sessionID in the
	// session's own workDir/env, mirroring stepengine.CheckRunner's return
	// shape (failedIdx=-1 means all passed). An unknown sessionID or empty
	// cmds returns (-1, 0, "", nil) — checks never block an advance the
	// spawner cannot run.
	RunStepChecks(ctx context.Context, sessionID string, cmds []string) (failedIdx, exitCode int, outputTail string, err error)
}

// ChainRunController lets agents set the next step's instructions/ticket in a
// workflow chain run. Nil-safe; guard with env.ChainRun == nil before calling.
type ChainRunController interface {
	SetNextStepInstructions(instanceID, instructions string) error
	SetNextStepTicket(instanceID, ticketID string) error
}

// SubworkflowState is the poll result returned by GetSubworkflow. Result and
// FailureReason are populated only for the completed/failed terminal
// statuses; Plan/Revision/Questions/Templates are populated only for the four
// plan-boundary statuses (planning/waiting_input/waiting_approval)
// — the polymorphism the plan-vs-terminal payload divergence belongs on this
// struct (and the orchestrator that fills it), not name-checks in the tool
// handlers (root CLAUDE.md rule 6). Templates is the bindable template library
// (service.PlanTemplateChoices): without it a caller cannot author the manifest
// revise_plan's `plan` argument takes, since `template` is the only field
// selecting a node's model and unknown names are rejected.
type SubworkflowState struct {
	Status        string          `json:"status"`
	Result        json.RawMessage `json:"result,omitempty"`
	FailureReason string          `json:"failure_reason,omitempty"`
	Plan          json.RawMessage `json:"plan,omitempty"`
	Revision      int             `json:"revision,omitempty"`
	Questions     json.RawMessage `json:"questions,omitempty"`
	Templates     json.RawMessage `json:"templates,omitempty"`
	PremiumCap    int             `json:"premium_cap,omitempty"`
}

// SubworkflowRunner starts callable workflows as detached project-scoped child
// runs and polls their status/result (async-with-poll contract), and drives the
// plan lifecycle (dynamic_workflow/revise_plan/approve_plan) for plan-driven
// children. Implemented by the orchestrator and injected via spawner.Config;
// the run_subworkflow/get_subworkflow/dynamic_workflow/revise_plan/approve_plan
// builtins call it. Nil-safe; guard with env.Subworkflows == nil.
type SubworkflowRunner interface {
	StartSubworkflow(ctx context.Context, parentInstanceID, projectID, workflow, instructions string) (instanceID string, err error)
	GetSubworkflow(ctx context.Context, callerInstanceID, projectID, instanceID, resultKey string) (SubworkflowState, error)

	// StartDynamicWorkflow starts the bundled plan-driven `dynamic` workflow (or
	// mode=auto variant) as a detached child, sharing StartSubworkflow's guards
	// (callable/purge/pause checks, depth cap, invocation budget, concurrency
	// slot, parent-death watcher) via the same helper.
	StartDynamicWorkflow(ctx context.Context, parentInstanceID, projectID, instructions, mode string) (instanceID string, err error)
	// RevisePlan and ApprovePlan drive a child's plan lifecycle on behalf of the
	// caller that started it (ownership enforced identically to GetSubworkflow).
	RevisePlan(ctx context.Context, callerInstanceID, projectID, instanceID string, req types.PlanReviseRequest) (*model.PlanRevision, error)
	ApprovePlan(ctx context.Context, callerInstanceID, projectID, instanceID string, revision int) (*model.PlanRevision, error)
}

// ToolEnv is the per-spawn environment threaded through every Invoke call.
// It carries the in-process services and identifiers handlers need to
// mirror the CLI socket flow without going over the network.
type ToolEnv struct {
	Pool               *db.Pool
	WSHub              service.WSHub
	Clock              clock.Clock
	SessionID          string
	AgentID            string
	AgentType          string
	ProjectID          string
	TicketID           string
	WorkflowName       string
	WorkflowInstanceID string
	// NodeID is the execution-identity slot (proc.nodeID) — the
	// agent_step_cursors cursor key, distinct from AgentType (the
	// agent_definitions template key). Set for every stepwise spawn.
	NodeID string
	// Steps is the spawner-owned rotation seam the complete_step builtin
	// uses (RotateSignals/NoteStepBoundary/RequestStepRotation). Nil-safe:
	// nil means no rotation policy (tests, console).
	Steps StepSession
	// ExternalID / ExternalContext mirror the workflow instance's external refs
	// (empty when unset). Threaded into kind=tool subprocess env as
	// NRF_EXTERNAL_ID / NRF_EXTERNAL_CONTEXT, matching spawner.prepareScriptSpawn.
	ExternalID      string
	ExternalContext string
	Findings        *service.FindingsService
	ProjectFindings *service.ProjectFindingsService
	Agent           *service.AgentService
	Workflow        *service.WorkflowService
	Ticket          *service.TicketService
	ArtifactSvc     *service.ArtifactService
	// DispatchRepo is required for tools that record dispatch rows (tools_python)
	// and for the toolAuditDecorator (registry.go wrapping). Nil-safe: both
	// skip Insert when nil.
	DispatchRepo *repo.DispatchRepo
	// Source discriminates which invoke site this ToolEnv is dispatched from
	// (model.DispatchSource* — mcp/http/console/engine); read by
	// toolAuditDecorator at Invoke time. Empty is treated as unattributed.
	Source string
	// SessionKind mirrors agent_sessions.kind for SessionID (workflow_agent/
	// console/console_chat), denormalized onto every recorded dispatch row.
	SessionKind string
	// WorkflowControl allows workflow_continue/workflow_fail builtins to act on the workflow.
	// Nil when the orchestrator is not wired (e.g. tests).
	WorkflowControl WorkflowController
	// Consultant allows the consult builtin to spawn a named consultant inline.
	// Nil when not wired (e.g. tests, or when agent is itself a consultant).
	Consultant ConsultantSpawner
	// Delegator allows the delegate/get_delegation builtins to spawn
	// tier-resolved workers downward. Nil when not wired (e.g. tests, or when
	// the delegate tool was stripped for this agent's tier/depth).
	Delegator Delegator
	// ChainRun lets the chain_next_* builtins set the next chain step's
	// instructions/ticket. Nil outside chain runs / in tests.
	ChainRun ChainRunController
	// Subworkflows starts/polls callable sub-workflows (run_subworkflow /
	// get_subworkflow builtins). Nil when not wired (tests, no orchestrator).
	Subworkflows SubworkflowRunner
	// Heartbeat bumps the calling agent's lastMessageTime so stall detection does
	// not kill it during a long blocking tool call. Nil-safe.
	Heartbeat func()
	// WorkDir is the agent/chat working directory: read_file/glob/grep
	// resolve relative paths against it but are not restricted to it,
	// edit_file/write_file/bash (tools_builtin/fs*.go) are jailed to it.
	// Empty means no filesystem access — those tools error.
	WorkDir string
	// FS holds per-session state for the native fs tools: the read-before-
	// edit/write tracking set and the background-shell registry (bash's
	// run_in_background + bash_output/kill_shell). Nil when not wired (e.g.
	// console sessions, tests) — handlers skip read-checks and background
	// tools error clearly when this is nil.
	FS *FSSession
	// SafetyCheck runs before every bash command when non-nil: returns
	// (allowed, reason, err). !allowed or a non-nil err both surface as a
	// blocking isError tool result, never a turn-fatal Go error. Nil = allow
	// (e.g. console sessions, tests).
	SafetyCheck func(command string) (allowed bool, reason string, err error)
}

// TerminalSignal is returned by handlers that end the runner loop.
// agent_fail / agent_continue / agent_callback all return this; the runner
// detects it via errors.As and short-circuits before issuing another turn.
type TerminalSignal struct {
	Status string // "FAIL", "CONTINUE", "CALLBACK"
	Reason string
	Level  int
}

// Error implements error so handlers can return TerminalSignal in the err slot.
func (t TerminalSignal) Error() string {
	return fmt.Sprintf("terminal:%s", t.Status)
}

// Registry maps tool name -> handler. Built per-spawn from the agent
// definition's tools CSV intersected with available builtins + HTTP defs.
type Registry map[string]ToolHandler
