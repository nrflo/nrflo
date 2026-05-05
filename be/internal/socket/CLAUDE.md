# Socket Package

Socket server for agent communication (Unix). Handles agent-facing methods only — all other operations go through the HTTP API.

## Transport

- **Unix socket** at `/tmp/nrflo/nrflo.sock` — used by agents

Clients connect via Unix socket.

## Protocol

The socket uses a **JSON-RPC style protocol** (line-delimited JSON).

### Request Format

All `findings.*` and `agent.*` requests require `instance_id` and `session_id` (set automatically from `NRF_WORKFLOW_INSTANCE_ID` and `NRF_SESSION_ID` env vars by the CLI). The service derives ticket, workflow, and agent_type from the session row — callers do not send them.

```json
{"method": "findings.add", "params": {"instance_id": "uuid", "session_id": "uuid", "key": "summary", "value": "..."}}
{"method": "agent.fail", "params": {"instance_id": "uuid", "session_id": "uuid", "reason": "..."}}
{"method": "agent.callback", "params": {"instance_id": "uuid", "session_id": "uuid", "level": 1}}
```

### Response Format

```json
{"success": true, "data": {...}}
{"success": false, "error": {"code": "NOT_FOUND", "message": "..."}}
```

## Supported Methods

| Method | Purpose |
|--------|---------|
| `findings.add` | Add a single finding |
| `findings.add-bulk` | Add multiple findings at once |
| `findings.get` | Get findings for an agent |
| `findings.append` | Append to an existing finding value |
| `findings.append-bulk` | Append multiple findings at once |
| `findings.delete` | Delete specific finding keys |
| `project_findings.add` | Add a single project-level finding |
| `project_findings.add-bulk` | Add multiple project-level findings |
| `project_findings.get` | Get project-level findings |
| `project_findings.append` | Append to a project-level finding |
| `project_findings.append-bulk` | Append multiple project-level findings |
| `project_findings.delete` | Delete project-level finding keys |
| `agent.fail` | Mark agent as failed; broadcasts with `session_id`, `model_id`, `result` |
| `agent.finished` | Mark agent as successfully finished (pass; proceed to next phase); broadcasts `agent.completed` with `session_id`, `model_id`, `result=pass` |
| `agent.continue` | Mark agent for context-exhaustion relaunch; broadcasts with `session_id`, `model_id` |
| `agent.callback` | Trigger callback to re-run earlier layer; broadcasts with `model_id`, `result` |
| `agent.context_update` | Update context_left for a session; no project required; broadcasts `agent.context_updated` with `session_id`, `context_left` |
| `agent.chain_next_instructions` | Set instructions for the next pending step in the workflow chain run associated with `instance_id`; requires `instance_id` and `instructions` params; returns `{status:"ok"}` |
| `agent.chain_next_ticket` | Set the ticket ID for the next pending ticket-scope step in the workflow chain run associated with `instance_id`; requires `instance_id` and `ticket_id` params; returns `{status:"ok"}` |
| `agent.record_event` | Record a Claude/codex hook event (PreToolUse/PostToolUse/UserPromptSubmit/Stop); no project required; inserts agent_messages row, broadcasts `messages.updated`. SessionEnd is silently ignored. Stop triggers a per-turn flush of new `event_msg/agent_message` records from the codex rollout JSONL (text category) so the model's spoken output is visible. |
| `workflow.skip` | Add a skip tag to a workflow instance; validates tag against workflow groups; broadcasts `skip_tag.added` |
| `ws.broadcast` | Broadcast event to WebSocket hub |
| `script.context` | Return the auto-injectable variable dict for a script-mode agent session; no project required; params: `{session_id}`; resolves session → workflow_instance → ticket (when ticket-scoped); returns 12-key dict: `session_id`, `instance_id`, `project_id`, `agent_type`, `workflow_id`, `scope_type`, `ticket_id`, `ticket_title`, `ticket_description`, `user_instructions` (string, "" if unset), `callback` (null or `{instructions,from_agent,level}`), `previous_data` (string, "" if no `to_resume` finding). |

## Common Tasks

### Changing Agent CLI Commands

The socket only handles agent-facing methods:

1. Update CLI command in `be/internal/cli/agent.go` or `findings.go`
2. Update socket handler in `handler.go`
3. Update service in `be/internal/service/`
4. Rebuild: `cd be && make build`
5. **Documentation updates:**

## Broadcast Helper

All socket handlers route WS broadcasts through `service.BroadcastFromCtx(hub, eventType, BroadcastCtx, data)` (defined in `be/internal/service/broadcast.go`). It is the single source of truth for the unpack-`BroadcastCtx`-and-broadcast pattern; the future API tool dispatcher (T4) calls the same helper. Do not reintroduce inline `ws.NewEvent(...)` + `hub.Broadcast(...)` pairs in socket handlers.

## Terminal Signal Dispatch

After the DB write and WS broadcast, the `agent.fail`, `agent.finished`, `agent.continue`, and `agent.callback` cases each dispatch a best-effort terminal signal through an injected `TerminalSignaler` (defined in `server.go`). This kills the running agent immediately so `monitorAll` exits its natural-exit wait and `handleCompletion` reads the DB-written result, eliminating the latency between the agent calling `nrflo agent fail/finished/continue/callback` and the spawner acting on it.

- **Interface**: four methods — `RequestTerminalSignal(projectID, ticketID, workflow, sessionID, result string) error`, `BumpLastMessage(projectID, ticketID, workflow, sessionID string) error`, `SetLastMessage(projectID, ticketID, workflow, sessionID, content string) error`, and `SignalSessionReady(sessionID string) error`.
- **Wiring**: `NewServerWithHub` accepts a `TerminalSignaler`; in production `cli/serve.go` passes `httpServer.GetOrchestrator()`; pass `nil` in tests.
- **Nil-safe**: `Handler` nil-guards before calling — passing `nil` disables the feature silently.
- **Order**: DB write → WS broadcast → terminal signal (best-effort, error is logged at INFO level and does not affect the response).
- **BumpLastMessage**: called by `agent.record_event` handler after inserting a hook message row. Forwards to `Orchestrator.BumpLastMessage` → `Spawner.BumpLastMessage`, which sends a session ID through `bumpMessageCh` so `monitorAll` updates `lastMessageTime`/`hasReceivedMessage` for the matching proc, preventing false-positive stall detection during active interactive CLI sessions.
- **SetLastMessage**: called alongside BumpLastMessage with the recorded message body. Forwards to `Orchestrator.SetLastMessage` → `Spawner.SetLastMessage`, which looks up the proc directly via `sessionProcs` and updates `proc.lastMessage` under `messagesMutex`. This makes the periodic "agent status" log line (`last_msg=...`) populate for interactive CLI agents — the PTY ferry drops raw bytes, so without this the status line was always empty in interactive mode.

## Files

| File | Purpose |
|------|---------|
| `server.go` | Socket listener, connection handling, `TerminalSignaler` interface |
| `handler.go` | Request routing and method dispatch |
| `handler_record_event.go` | `agent.record_event` handler: PreToolUse/PostToolUse → DB insert + WS broadcast + stall bump; opportunistic codex context update via `extractCodexContextLeft`; Stop → `flushCodexAgentMessages` to emit new agent text rows |
| `handler_codex_context.go` | Codex JSONL extractors: `extractCodexContextLeft` (latest `token_count` → % context remaining) and `extractCodexNewAgentMessages` (new `event_msg/agent_message` bodies since the per-session offset). Reasoning blocks are NOT extracted — codex 0.125 emits only encrypted reasoning. |
| `protocol.go` | JSON-RPC protocol types (Request, Response, Error) |
| `handler_script_context.go` | `script.context` handler: resolve session → wfi → ticket, assemble 12-key context dict |
| `handler_terminal_signal_test.go` | Terminal signal dispatch: fail/continue/callback dispatch, best-effort error handling, nil-guard |
