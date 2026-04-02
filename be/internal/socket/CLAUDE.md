# Socket Package

Socket server for agent communication (Unix). Handles agent-facing methods only — all other operations go through the HTTP API.

## Transport

- **Unix socket** at `/tmp/nrflow/nrflow.sock` — used by agents

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
| `agent.continue` | Mark agent for continuation; broadcasts with `session_id`, `model_id` |
| `agent.callback` | Trigger callback to re-run earlier layer; broadcasts with `model_id`, `result` |
| `agent.context_update` | Update context_left for a session; no project required; broadcasts `agent.context_updated` with `session_id`, `context_left` |
| `workflow.skip` | Add a skip tag to a workflow instance; validates tag against workflow groups; broadcasts `skip_tag.added` |
| `ws.broadcast` | Broadcast event to WebSocket hub |

## Common Tasks

### Changing Agent CLI Commands

The socket only handles agent-facing methods:

1. Update CLI command in `be/internal/cli/agent.go` or `findings.go`
2. Update socket handler in `handler.go`
3. Update service in `be/internal/service/`
4. Rebuild: `cd be && make build`
5. **Documentation updates:**
   - `guidelines/agent-protocol.md` — if agent-facing commands change

## Files

| File | Purpose |
|------|---------|
| `server.go` | Socket listener, connection handling |
| `handler.go` | Request routing and method dispatch |
| `protocol.go` | JSON-RPC protocol types (Request, Response, Error) |
