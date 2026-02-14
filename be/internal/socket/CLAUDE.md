# Socket Package

Unix socket server for agent communication. Handles agent-facing methods only — all other operations go through the HTTP API.

## Protocol

The socket uses a **JSON-RPC style protocol** (line-delimited JSON) at `/tmp/nrworkflow/nrworkflow.sock`.

### Request Format

```json
{"method": "findings.add", "params": {"ticket_id": "T-1", "agent_type": "analyzer", "key": "summary", "value": "..."}}
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
| `agent.complete` | Mark agent as completed (pass) |
| `agent.fail` | Mark agent as failed |
| `agent.continue` | Mark agent for continuation |
| `agent.callback` | Trigger callback to re-run earlier layer |
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
