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
	Findings           *service.FindingsService
	ProjectFindings    *service.ProjectFindingsService
	Agent              *service.AgentService
	Workflow           *service.WorkflowService
	ArtifactSvc        *service.ArtifactService
	// DispatchRepo is required for tools that record dispatch rows (tools_http, tools_python).
	// Nil-safe: handlers skip Insert when nil.
	DispatchRepo       *repo.DispatchRepo
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
