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
| `global.claude_limits_update` | Update Claude API rate limit state; broadcasts `global.claude_limits_updated` |
| `script.context` | Return 12-key auto-injectable dict for script-mode session. Params: `{session_id}` |

All `findings.*` and `agent.*` requests require `instance_id` and `session_id` (set from `NRF_WORKFLOW_INSTANCE_ID`/`NRF_SESSION_ID` env vars by the CLI).

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
| `server.go` | Socket listener, connection handling, `TerminalSignaler` interface |
| `handler.go` | Request routing and method dispatch |
| `handler_record_event.go` | `agent.record_event`: PreToolUse → DB insert + WS broadcast; Stop → codex JSONL flush |
| `handler_codex_context.go` | Codex JSONL extractors: context left + new agent messages |
| `protocol.go` | JSON-RPC protocol types (Request, Response, Error) |
| `handler_script_context.go` | `script.context` handler |
| `handler_agent_log.go` | `agent.log` handler |
