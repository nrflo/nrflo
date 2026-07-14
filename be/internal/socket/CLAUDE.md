# Socket Package

Socket server for agent communication (Unix). Handles agent-facing methods only — all other operations go through the HTTP API. Deep mechanics: [REFERENCE.md](REFERENCE.md) — read it before changing method payloads, observer authorization, terminal-signal dispatch, or handler layout.

## Transport

Unix socket at `$NRFLO_HOME/agent.sock` (override `NRFLO_SOCKET`). Eagerly bound at server startup via `BindListener()` before the HTTP listener. Protocol: line-delimited JSON-RPC; see `protocol.go` for `Request`, `Response`, and `Error` types.

## Supported Methods

Methods by namespace; the full per-method params/behavior table is [REFERENCE.md § Supported Methods](REFERENCE.md#supported-methods) — read it before adding or changing a method:

- `findings.*` / `project_findings.*` — add / add-bulk / get / append / append-bulk / delete, plus `findings.emit` (schema-validated store)
- `agent.*` — lifecycle (`fail`/`finished`/`continue`/`callback`), `context_update`, `rate_limits_update`, `chain_next_instructions`/`chain_next_ticket`, `consult` (sync api-mode consultant), `record_event` (Claude hook events: tool spans, stop-hook completion enforcement, transcript thinking tail), `log`
- `workflow.*` — skip / continue / fail (session-ownership validated)
- `ws.broadcast`, `script.context` (script-mode context dict), `artifact.add/list/get`, `tools.list/call` (api-via-cli in-process tool registry via the wired `ToolDispatcher`)
- `observer.*` — workflow/project/global reads plus feature-gated mutate methods

All `findings.*` and `agent.*` requests require `instance_id` and `session_id` (set from `NRF_WORKFLOW_INSTANCE_ID`/`NRF_SESSION_ID` env vars by the CLI). `agent.consult` and `observer.workflow.trigger`/`retry_failed` need a `WorkflowOrchestrator` wired via `Server.SetWorkflowRunner()`.

## Observer Authorization

Every `observer.*` call is authorized against the calling session (`kind=observer`); scope precedence (workflow < project < global) gates which namespaces are callable, and mutate methods re-check `experimental_observer_enabled` at call time. Full rule chain incl. the `__global__` mutation deny: [REFERENCE.md § Observer Authorization](REFERENCE.md#observer-authorization) — read before touching `handler_observer*.go`.

## Broadcast Helper

All socket handlers route WS broadcasts through `service.BroadcastFromCtx(hub, eventType, BroadcastCtx, data)` (`be/internal/service/broadcast.go`). Do not reintroduce inline `ws.NewEvent(...)` + `hub.Broadcast(...)` pairs in socket handlers.

## Terminal Signal Dispatch

`agent.fail`/`agent.finished`/`agent.continue`/`agent.callback` dispatch a best-effort terminal signal through an injected `TerminalSignaler` (`server_interfaces.go`); nil-safe, order: DB write → WS broadcast → signal. Interface methods and wiring: [REFERENCE.md § Terminal Signal Dispatch](REFERENCE.md#terminal-signal-dispatch) — read before changing lifecycle handlers or stall detection inputs.

## Files

Listener + `Handler` + dispatcher setters in `server.go`; interfaces in `server_interfaces.go` (json/primitives only — keeps socket free of apirun imports); routing in `handler.go`; one `handler_<namespace>*.go` per method namespace. Per-file inventory: [REFERENCE.md § Files](REFERENCE.md#files).

`make test-pkg PKG=socket`.
