package apirun

import (
	"context"
	"encoding/json"
	"fmt"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/repo"
	"be/internal/service"
	"be/internal/spawner/apirun/provider"
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

// ChainRunController lets agents set the next step's instructions/ticket in a
// workflow chain run. Nil-safe; guard with env.ChainRun == nil before calling.
type ChainRunController interface {
	SetNextStepInstructions(instanceID, instructions string) error
	SetNextStepTicket(instanceID, ticketID string) error
}

// DeepResearchRunner runs the deep-research workflow synchronously under the
// given project and returns its `report` finding as raw JSON. Implemented by the
// orchestrator and injected via spawner.Config; the web_deep_research builtin
// calls it. Nil-safe; guard with env.DeepResearch == nil before calling.
type DeepResearchRunner interface {
	RunDeepResearch(ctx context.Context, projectID, question string) (report json.RawMessage, err error)
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
	// DispatchRepo is required for tools that record dispatch rows (tools_python).
	// Nil-safe: handlers skip Insert when nil.
	DispatchRepo *repo.DispatchRepo
	// WorkflowControl allows workflow_continue/workflow_fail builtins to act on the workflow.
	// Nil when the orchestrator is not wired (e.g. tests).
	WorkflowControl WorkflowController
	// Consultant allows the consult builtin to spawn a named consultant inline.
	// Nil when not wired (e.g. tests, or when agent is itself a consultant).
	Consultant ConsultantSpawner
	// ChainRun lets the chain_next_* builtins set the next chain step's
	// instructions/ticket. Nil outside chain runs / in tests.
	ChainRun ChainRunController
	// DeepResearch runs the deep-research workflow synchronously (web_deep_research
	// builtin). Nil when not wired (tests, or contexts without an orchestrator).
	DeepResearch DeepResearchRunner
	// Heartbeat bumps the calling agent's lastMessageTime so stall detection does
	// not kill it during a long blocking tool call. Nil-safe.
	Heartbeat func()
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
