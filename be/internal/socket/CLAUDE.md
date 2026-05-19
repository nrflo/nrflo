# Socket Package

Socket server for agent communication (Unix). Handles agent-facing methods only — all other operations go through the HTTP API.

## Transport

Unix socket at `$NRFLO_HOME/agent.sock` (override `NRFLO_SOCKET`). Eagerly bound at server startup via `BindListener()` before the HTTP listener. Protocol: line-delimited JSON-RPC; see `protocol.go` for `Request`, `Response`, and `Error` types.

## Supported Methods

| Method | Purpose |
|--------|---------|
| `findings.add` | Add a single finding |
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
| `agent.chain_next_instructions` | Set instructions for the next pending chain step |
| `agent.chain_next_ticket` | Set ticket ID for the next pending ticket-scope chain step |
| `agent.record_event` | Record Claude/codex hook event; PreToolUse inserts message row + WS broadcast; PostToolUse no-op; Stop flushes codex JSONL messages |
| `agent.log` | Insert `agent_messages` row from script agent. Params: `{session_id, type?, message, payload?}` |
| `workflow.skip` | Add skip tag to workflow instance; validates against workflow groups |
| `ws.broadcast` | Broadcast event to WebSocket hub |
| `script.context` | Return 13-key auto-injectable dict for script-mode session (incl. `seed_findings`). Params: `{session_id}` |
| `artifact.add` | Upload artifact inline (base64); max 32 MiB; broadcasts `artifact.created`. Params: `{session_id, name, content_b64, content_type?}` |
| `artifact.list` | List artifacts for the session's workflow instance. Params: `{session_id}` |
| `artifact.get` | Materialize artifact to stage dir and return abs path. Params: `{session_id, name}` |
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

`observer.workflow.trigger` and `observer.workflow.retry_failed` require a `WorkflowOrchestrator` wired via `Server.SetWorkflowRunner()` (nil → internal error response).

## Broadcast Helper

All socket handlers route WS broadcasts through `service.BroadcastFromCtx(hub, eventType, BroadcastCtx, data)` (`be/internal/service/broadcast.go`). Do not reintroduce inline `ws.NewEvent(...)` + `hub.Broadcast(...)` pairs in socket handlers.

## Terminal Signal Dispatch

After the DB write and WS broadcast, `agent.fail`, `agent.finished`, `agent.continue`, and `agent.callback` dispatch a best-effort terminal signal through an injected `TerminalSignaler`:

- Interface: `RequestTerminalSignal`, `BumpLastMessage`, `SetLastMessage`, `SignalSessionReady` (defined in `server.go`).
- Wiring: production `cli/serve.go` passes `httpServer.GetOrchestrator()` as signaler; `nil` in tests.
- Nil-safe; order: DB write → WS broadcast → terminal signal (best-effort, error logged at INFO).
- `BumpLastMessage`: updates `lastMessageTime`/`hasReceivedMessage` to prevent false-positive stall detection.
- `SetLastMessage`: updates `proc.lastMessage` under `messagesMutex`; populates "agent status" log line for interactive CLI agents.

## Files

| File | Purpose |
|------|---------|
| `server.go` | Socket listener, connection handling, `TerminalSignaler` and `WorkflowOrchestrator` interfaces |
| `handler.go` | Request routing and method dispatch |
| `handler_record_event.go` | `agent.record_event`: PreToolUse → DB insert + WS broadcast; Stop → codex JSONL flush |
| `handler_gemini_normalize.go` | In-place rename of Gemini hook event names to Claude equivalents before the switch |
| `handler_codex_context.go` | Codex JSONL extractors: context left + new agent messages |
| `protocol.go` | JSON-RPC protocol types (Request, Response, Error) |
| `handler_script_context.go` | `script.context` handler |
| `handler_agent_log.go` | `agent.log` handler |
| `handler_artifact.go` | `artifact.add/list/get` handlers |
| `handler_observer.go` | `observer.*` dispatch: `methodSpec` map, `authorizeObserver` helper, namespace routing |
| `handler_observer_workflow.go` | `observer.workflow.*` handlers: show/runs/findings/logs/trigger/retry_failed/def.update |
| `handler_observer_project.go` | `observer.project.*` handlers: workflows/runs/findings/env.list/env.set/env.unset/workflow.create/workflow.delete |
| `handler_observer_global.go` | `observer.global.*` handlers: projects/recent_sessions/health/project.create/project.delete |
