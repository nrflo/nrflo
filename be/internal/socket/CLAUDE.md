# Socket Package

Socket server for agent communication (Unix). Handles agent-facing methods only — all other operations go through the HTTP API. Deep mechanics: [REFERENCE.md](REFERENCE.md) — read it before changing method payloads, observer authorization, terminal-signal dispatch, or handler layout.

## Transport

Unix socket at `$NRFLO_HOME/agent.sock` (override `NRFLO_SOCKET`). Eagerly bound at server startup via `BindListener()` before the HTTP listener. Protocol: line-delimited JSON-RPC; see `protocol.go` for `Request`, `Response`, and `Error` types.

## Supported Methods

Methods by namespace; the full per-method params/behavior table is [REFERENCE.md § Supported Methods](REFERENCE.md#supported-methods) — read it before adding or changing a method:

- `findings.*` / `project_findings.*` — add / add-bulk / get / append / append-bulk / delete, plus `findings.emit` (schema-validated store)
- `agent.*` — lifecycle (`fail`/`finished`/`continue`/`callback`), `context_update`, `rate_limits_update`, `chain_next_instructions`/`chain_next_ticket`, `consult` (sync api-mode consultant), `record_event` (Claude hook events: tool spans, stop-hook completion enforcement, transcript thinking tail; for a `kind='console'` session, `PreToolUse` additionally blocks on a `ConsoleHooks`-routed human approval decision, returned as `permission_decision` in the response; `UserPromptSubmit` returns `hookSpecificOutput.additionalContext` for console-kind sessions via the optional, nil-safe `ContextInjector`), `log`; Notification hook rows default to category `model.MsgCategorySystemNotice` (`system_notice`), and a `UserPromptSubmit` prompt whose trimmed text starts with `<task-notification>` (the CLI harness's backgrounded `get_delegation` echo) records under `model.MsgCategoryTaskNotification` (`task_notification`) instead of `user_input`
- `workflow.*` — skip / continue / fail (session-ownership validated)
- `console.session` — mint a local tool session; `console.catalog` discovers available engines/models and live chats; `console.chat` starts a chat; `console.attach` returns an existing live chat's unchanged scoped bearer; `console.close` closes/kills a live chat by id. All resolve project explicit-hint → `cwd` → global
- `ws.broadcast`, `script.context` (script-mode context dict), `artifact.add/list/get`, `tools.list/call` (api-via-cli in-process tool registry via the wired `ToolDispatcher`)
- `observer.*` — workflow/project/global reads plus feature-gated mutate methods

All `findings.*` and `agent.*` requests require `instance_id` and `session_id` (set from `NRF_WORKFLOW_INSTANCE_ID`/`NRF_SESSION_ID` env vars by the CLI). `agent.consult` and `observer.workflow.trigger`/`retry_failed` need a `WorkflowOrchestrator` wired via `Server.SetWorkflowRunner()`. A `Notification` hook classified as idle-waiting or permission-prompt additionally fires an immediate idle nudge via `TerminalSignaler.TriggerIdleNudge` for autonomous `kind='workflow_agent'`, `cli_interactive` claude sessions only. Every `record_event` is logged exactly once — INFO for lifecycle events, DEBUG for high-volume ones, WARN for rejected/unrecognized ones — see [REFERENCE.md § Supported Methods](REFERENCE.md#supported-methods).

## Observer Authorization

Every `observer.*` call is authorized against the calling session (`kind=observer`); scope precedence (workflow < project < global) gates which namespaces are callable, and mutate methods re-check `experimental_observer_enabled` at call time. Full rule chain incl. the `__global__` mutation deny: [REFERENCE.md § Observer Authorization](REFERENCE.md#observer-authorization) — read before touching `handler_observer*.go`.

## Broadcast Helper

All socket handlers route WS broadcasts through `service.BroadcastFromCtx(hub, eventType, BroadcastCtx, data)` (`be/internal/service/broadcast.go`). Do not reintroduce inline `ws.NewEvent(...)` + `hub.Broadcast(...)` pairs in socket handlers.

## Terminal Signal Dispatch

`agent.fail`/`agent.finished`/`agent.continue`/`agent.callback` dispatch a best-effort terminal signal through an injected `TerminalSignaler` (`server_interfaces.go`); nil-safe, order: DB write → WS broadcast → signal. Interface methods and wiring: [REFERENCE.md § Terminal Signal Dispatch](REFERENCE.md#terminal-signal-dispatch) — read before changing lifecycle handlers or stall detection inputs.

## Files

Listener + `Handler` in `server.go`, dependency wiring in `server_setters.go`, and interfaces in `server_interfaces.go`; routing lives in `handler.go` plus `handler_<namespace>*.go`. Per-file inventory: [REFERENCE.md § Files](REFERENCE.md#files).

`make test-pkg PKG=socket`.
