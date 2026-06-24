# Socket Package

Socket server for agent communication (Unix). Handles agent-facing methods only — all other operations go through the HTTP API.

## Transport

Unix socket at `$NRFLO_HOME/agent.sock` (override `NRFLO_SOCKET`). Eagerly bound at server startup via `BindListener()` before the HTTP listener. Protocol: line-delimited JSON-RPC; see `protocol.go` for `Request`, `Response`, and `Error` types.

## Supported Methods

| Method | Purpose |
|--------|---------|
| `findings.add` | Add a single finding |
| `findings.emit` | Validate `{key, value}` against the workflow's configured finding schema for the key, then store it; rejects (validation error with the expected-structure example) on mismatch or unknown key |
| `findings.add-bulk` | Add multiple findings at once |
| `findings.get` | Get findings; `layer` int returns `{agent_type: findings\|null}` map (mutually exclusive with `agent_type`) |
| `findings.append` | Append to an existing finding value |
| `findings.append-bulk` | Append multiple findings |
| `findings.delete` | Delete specific finding keys |
| `project_findings.add` | Add a single project-level finding |
| `project_findings.add-bulk` | Add multiple project-level findings |
| `project_findings.get` | Get project-level findings |
| `project_findings.append` | Append to a project-level finding |
| `project_findings.append-bulk` | Append multiple project-level findings |
| `project_findings.delete` | Delete project-level finding keys |
| `agent.fail` | Mark agent as failed; broadcasts `agent.completed` |
| `agent.finished` | Mark agent as successfully finished; broadcasts `agent.completed` (result=pass) |
| `agent.continue` | Mark agent for context-exhaustion relaunch |
| `agent.callback` | Trigger callback to re-run earlier layer |
| `agent.context_update` | Update `context_left` for a session; broadcasts `agent.context_updated` |
| `agent.rate_limits_update` | Record Claude subscription rate-limit windows (`five_hour`/`seven_day`: `used_percentage`+`resets_at`) for a session; persists the latest near-exhausted future reset to `rate_limit_reset_ts`; broadcasts `agent.rate_limits_updated`. Params: `{session_id, five_hour?, seven_day?}` |
| `agent.chain_next_instructions` | Set instructions for the next pending chain step |
| `agent.chain_next_ticket` | Set ticket ID for the next pending ticket-scope chain step |
| `agent.consult` | Synchronously spawn an api-mode consultant under the caller workflow instance and return its answer. Params: `{session_id, consultant, question}`. Requires `WorkflowOrchestrator` wired via `Server.SetWorkflowRunner()` (nil → internal error). |
| `agent.record_event` | Record Claude hook event; PreToolUse (invoke) + PostToolUse (`[tool] → output` result) insert message rows + WS broadcast. PostToolUse for `spawner.IsHiddenResultTool` tools (Read/Bash/Edit) is dropped at the source (`status="ignored"`, no row) — the invoke row already names the file/command. Stop enforces completion (`handleStopHook`): for an autonomous `running` session with no completion-tool result, returns a `decision:block` + finish-reminder (up to `stopBlockCap` blocks) so the CLI keeps the agent going, then fails it (`unresponsive_after_stop_blocks`); sessions with a result or non-running status pass through. PreToolUse/PreCompact also tail the Claude transcript (`event["transcript_path"]`) into `category="thinking"` rows — see `transcript_thinking.go` |
| `agent.log` | Insert `agent_messages` row from script agent. Params: `{session_id, type?, message, payload?}` |
| `workflow.skip` | Add skip tag to workflow instance; validates against workflow groups |
| `workflow.continue` | Resume a paused (waiting) workflow instance. Params: `{session_id, instance_id, instructions?}`; validates session ownership |
| `workflow.fail` | Fail a workflow instance with a reason. Params: `{session_id, instance_id, reason}`; validates session ownership |
| `ws.broadcast` | Broadcast event to WebSocket hub |
| `script.context` | Return 19-key auto-injectable dict for script-mode session (incl. `seed_findings`, `workflow_status`, `workflow_result`, `workflow_final_result`, `failure_reason`, `external_id`, `external_context`). Params: `{session_id}` |
| `artifact.add` | Upload artifact inline (base64); max 32 MiB; broadcasts `artifact.created`. Params: `{session_id, name, content_b64, content_type?}` |
| `artifact.list` | List artifacts for the session's workflow instance. Params: `{session_id}` |
| `artifact.get` | Materialize artifact to stage dir and return abs path. Params: `{session_id, name}` |
| `tools.list` | MCP tool list for an api-via-cli session's in-process registry; returns the tools array (`name`/`description`/`inputSchema`). Params: `{session_id, instance_id}`. Requires a `ToolDispatcher` wired via `Server.SetToolDispatcher()` (nil → internal error) |
| `tools.call` | Dispatch a tool call to the session registry; returns `{output, is_error}` (terminal signals fire `RequestTerminalSignal` server-side). Params: `{session_id, instance_id, name, input}` |
| `observer.workflow.show` | Get workflow definition. Params: `{session_id, project_id?, workflow_id?}` |
| `observer.workflow.runs` | List workflow instances for the attached workflow. Params: `{session_id, project_id?, workflow_id?}` |
| `observer.workflow.findings` | Get findings for the attached workflow instance. Params: `{session_id, instance_id?}` |
| `observer.workflow.logs` | Get agent messages for the most recent (or specified) session. Params: `{session_id, target_session_id?, limit?, offset?}` |
| `observer.workflow.trigger` | Start a workflow run (mutate; requires observer enabled). Params: `{session_id, ticket_id?, instructions?, scope_type?}` |
| `observer.workflow.retry_failed` | Retry failed workflow from failed layer (mutate). Params: `{session_id, target_session_id?}` |
| `observer.workflow.def.update` | Update workflow definition (mutate). Params: `{session_id, ...WorkflowDefUpdateRequest}` |
| `observer.project.workflows` | List workflow definitions for a project. Params: `{session_id, project_id?}` |
| `observer.project.runs` | List project-scoped workflow instances. Params: `{session_id, project_id?}` |
| `observer.project.findings` | Get project findings. Params: `{session_id, project_id?}` |
| `observer.project.env.list` | List project env vars. Params: `{session_id, project_id?}` |
| `observer.project.env.set` | Upsert project env var (mutate). Params: `{session_id, project_id?, name, value}` |
| `observer.project.env.unset` | Delete project env var (mutate). Params: `{session_id, project_id?, name}` |
| `observer.project.workflow.create` | Create workflow definition (mutate). Params: `{session_id, project_id?, ...WorkflowDefCreateRequest}` |
| `observer.project.workflow.delete` | Delete workflow definition (mutate). Params: `{session_id, project_id?, workflow_id}` |
| `observer.global.projects` | List all projects. Params: `{session_id}` |
| `observer.global.recent_sessions` | List recent agent sessions for a project. Params: `{session_id, project_id?, limit?}` |
| `observer.global.health` | DB ping + observer feature flag status. Params: `{session_id}` |
| `observer.global.project.create` | Create a project (mutate). Params: `{session_id, project_id, name?, root_path?, default_branch?}` |
| `observer.global.project.delete` | Delete a project (mutate). Params: `{session_id, project_id}` |

All `findings.*` and `agent.*` requests require `instance_id` and `session_id` (set from `NRF_WORKFLOW_INSTANCE_ID`/`NRF_SESSION_ID` env vars by the CLI).

## Observer Authorization

All `observer.*` methods require `session_id` in params identifying the calling observer session. The handler enforces:
1. `kind=observer` on the session row
2. Scope precedence: workflow-scoped observer can only call `observer.workflow.*`; project-scoped can call `workflow.*` and `project.*`; global can call everything
3. Project-scoped observer: `project_id` in params must match session's project
4. Workflow-scoped observer: `workflow_id` in params must match session's workflow instance
5. Mutate methods additionally re-check `experimental_observer_enabled` at call time
6. `observer.*` workflow-def create/update/delete reject the reserved `__global__` project via `denyGlobalWorkflowMutation` (`handler_observer.go`) — global workflow defs are admin-HTTP-only, never mutable from the agent socket

`observer.workflow.trigger` and `observer.workflow.retry_failed` require a `WorkflowOrchestrator` wired via `Server.SetWorkflowRunner()` (nil → internal error response).

## Broadcast Helper

All socket handlers route WS broadcasts through `service.BroadcastFromCtx(hub, eventType, BroadcastCtx, data)` (`be/internal/service/broadcast.go`). Do not reintroduce inline `ws.NewEvent(...)` + `hub.Broadcast(...)` pairs in socket handlers.

## Terminal Signal Dispatch

After the DB write and WS broadcast, `agent.fail`, `agent.finished`, `agent.continue`, and `agent.callback` dispatch a best-effort terminal signal through an injected `TerminalSignaler`:

- Interface: `RequestTerminalSignal`, `BumpLastMessage`, `SetLastMessage`, `SignalSessionReady` (defined in `server_interfaces.go`).
- Wiring: production `cli/serve.go` passes `httpServer.GetOrchestrator()` as signaler; `nil` in tests.
- Nil-safe; order: DB write → WS broadcast → terminal signal (best-effort, error logged at INFO).
- `BumpLastMessage`: updates `lastMessageTime`/`hasReceivedMessage` to prevent false-positive stall detection.
- `SetLastMessage`: updates `proc.lastMessage` under `messagesMutex`; populates "agent status" log line for interactive CLI agents.

## Files

| File | Purpose |
|------|---------|
| `server.go` | Socket listener, connection handling, `Handler` struct, dispatcher setters (`SetWorkflowRunner`/`SetToolDispatcher`) |
| `server_interfaces.go` | `WorkflowOrchestrator`, `ToolDispatcher`, `TerminalSignaler` interfaces (json/primitives only; keeps socket free of apirun imports) |
| `handler.go` | Request routing and method dispatch |
| `handler_findings.go` | `findings.*` handlers (add/add-bulk/get/append/append-bulk/delete) |
| `handler_tools.go` | `tools.list`/`tools.call` handlers — proxy to the wired `ToolDispatcher` |
| `handler_record_event.go` | `agent.record_event`: PreToolUse (invoke) + PostToolUse (result) → DB insert + WS broadcast; Stop → no-op boundary ack |
| `handler_record_event_parse.go` | `tool_response`/`tool_result` body extraction (incl. MCP content arrays) |
| `transcript_thinking.go` | Best-effort tail of Claude transcript JSONL → `thinking` rows, gated by `capture_thinking_enabled`; per-session byte offsets on `Handler` behind `thinkingMu`, inserted before the tool row so they render above it |
| `protocol.go` | JSON-RPC protocol types (Request, Response, Error) |
| `handler_script_context.go` | `script.context` handler |
| `handler_agent_log.go` | `agent.log` handler |
| `handler_artifact.go` | `artifact.add/list/get` handlers |
| `handler_observer.go` | `observer.*` dispatch: `methodSpec` map, `authorizeObserver` helper, namespace routing |
| `handler_observer_workflow.go` | `observer.workflow.*` handlers: show/runs/findings/logs/trigger/retry_failed/def.update |
| `handler_observer_project.go` | `observer.project.*` handlers: workflows/runs/findings/env.list/env.set/env.unset/workflow.create/workflow.delete |
| `handler_observer_global.go` | `observer.global.*` handlers: projects/recent_sessions/health/project.create/project.delete |
