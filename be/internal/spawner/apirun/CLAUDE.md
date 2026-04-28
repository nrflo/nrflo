# apirun Package

In-process tool-use loop that drives API-mode agents through one or more provider turns. Replaces the CLI exec-and-parse path for agents whose `agent_definitions.execution_mode='api'`.

## Components

| File | Responsibility |
|------|----------------|
| `runner.go` | Turn loop: build initial user message, call `Provider.Run`, handle `end_turn`/`tool_use`/`max_tokens`, dispatch tools, append assistant + user messages, loop. |
| `interfaces.go` | Small surfaces consumed by the runner: `MessageSink`, `ProcState`, `AgentSvc`, `ErrorRecorder`. Spawner supplies adapters wrapping `*processInfo` so apirun never imports spawner. |
| `tool.go` | `ToolHandler` interface, `ToolEnv` struct, `TerminalSignal` (FAIL/CONTINUE/CALLBACK), `Registry` type alias. |
| `registry.go` | `ResolveRegistry(toolsCSV, builtins, httpDefs, httpFactory)` — glob-matches CSV against builtins ∪ HTTP defs and returns specs+handlers map. |
| `secret_resolver.go` | `DereferenceSecretRef("env:NAME" / "file:/path" / "literal:VALUE")` — shared by anthropic credentials and HTTP tool bearer auth. |
| `sink.go` | Coalescing `provider.EventSink` adapter that flushes text deltas to `MessageSink` on idle (200ms) or buffer overflow (80 chars). |
| `errors.go` | `classifyProviderError` mapping HTTP status / parse errors to (status, system message). |
| `provider/` | Provider abstraction (`Provider`, `Request`, `EventSink`, `FinalResponse`). Anthropic streaming impl + mock for tests. |
| `tools_builtin/` | Go-builtin tool handlers (findings.*, project_findings.*, agent_*, workflow_skip). Wraps existing services + `service.BroadcastFromCtx` for WS parity with the socket handler. |
| `tools_http/` | Generic HTTP tool handler driven by `model.ToolDefinition`. Bearer auth (env / secret_ref), 30s default timeout, 5xx-retry-once, 16KB body cap. |

## Tool Dispatch Flow

```
Provider.Run → resp.StopReason
  ├── end_turn         → SetFinalStatus("PASS")
  ├── max_tokens/...   → fail with system message
  └── tool_use:
        for block in resp.Content where block.Type=="tool_use":
            handler := registry[block.ToolName]
            out, isErr, err := handler.Invoke(ctx, env, block.Input)
            if errors.As(err, &TerminalSignal):
                proc.SetFinalStatus(ts.Status)        # FAIL/CONTINUE/CALLBACK
                if CALLBACK: proc.SetCallbackLevel(level)
                return                                 # short-circuit loop
            results.append(tool_result{output:out, is_error:isErr})
        msgs.append(assistant=resp.Content)
        msgs.append(user=results)
        continue                                       # next turn
```

Sequential dispatch only in v1 (`runner.dispatchTools`). The for-range loop is the natural slot for parallel dispatch (TODO comment in code).

## Terminal Signals

Handlers self-declare terminal status by returning a `TerminalSignal` in the `err` slot. Runner detects via `errors.As`, sets `proc.finalStatus`, and exits before issuing another provider turn. Adding a new terminal tool requires no runner change.

| Signal Status | Set By | Triggers |
|---------------|--------|----------|
| `FAIL` | `agent_fail` | spawner registers stop with result=fail; finalizePhase does not pass |
| `CONTINUE` | `agent_continue` | monitorAll calls `relaunchForContinuation` for the next session |
| `CALLBACK` | `agent_callback` | finalizePhase reads `callback_level` finding and returns `CallbackError` |

Each terminal handler also calls the corresponding `AgentService` method first, so the DB row + WS broadcast happen identically to CLI agents (which call the same services via the Unix socket).

## Builtins (17 handlers)

| Tool name | Service call | WS event |
|-----------|-------------|----------|
| `findings_add` / `findings_add_bulk` / `findings_append` / `findings_append_bulk` / `findings_get` / `findings_delete` | `FindingsService.*` | `findings.updated` |
| `project_findings_add` / `..._add_bulk` / `..._append` / `..._append_bulk` / `..._get` / `..._delete` | `ProjectFindingsService.*` | `project_findings.updated` |
| `agent_fail` / `agent_continue` / `agent_callback` | `AgentService.{Fail,Continue,Callback}` | `agent.completed` / `agent.continued` / `agent.completed` |
| `agent_context_update` | `AgentService.UpdateContextLeft` | `agent.context_updated` |
| `workflow_skip` | `WorkflowService.AddSkipTag` | `skip_tag.added` |

`tools_builtin/builtins.go` exposes the canonical map via `Builtins()` for the registry resolver.

## HTTP Tool Handler

`tools_http.New(client) apirun.HTTPHandlerFactory` returns a factory bound to a shared `http.Client`. Each handler:

1. POSTs `{"tool":<name>,"input":<input>,"context":{"project_id","workflow","session_id"}}` to `def.Endpoint`.
2. Per-request timeout = `def.TimeoutSec` seconds (default 30s).
3. Auth header per `def.AuthMethod`:
   - `none` — no header
   - `bearer_env` — `Authorization: Bearer ${ENV[def.AuthRef]}`
   - `bearer_secret_ref` — `Authorization: Bearer <DereferenceSecretRef(def.AuthRef)>`
4. 5xx → wait 500ms, retry once. 4xx → return immediately with `isError=true`.
5. Response body capped at 16 KB; truncated bodies get a ` ... [truncated]` suffix.

## Per-Agent Registry Resolution

`apirun.ResolveRegistry(toolsCSV, builtins, httpDefs, httpFactory)`:

- `""` (empty CSV) → empty registry; agent runs text-only (T3 path).
- `"findings.*"` → all six findings builtins (matcher: `*` and `prefix.*`).
- `"agent_*,workflow_skip"` → all four agent_* + workflow_skip.
- `"*"` → every builtin + every in-scope HTTP tool.
- Exact name → only that handler.
- No matches for any pattern → spawn fails with `no tools matched pattern "..."` (config error).
- Builtin name collision with HTTP `def.Name` → spawn fails with collision error.

HTTP defs in scope: `project_id IS NULL OR project_id == agent.project_id` AND `workflow_id IS NULL OR workflow_id == agent.workflow_name`. Project filter happens in the repo (`ListByProject`); workflow filter happens in `spawner.loadAPIHTTPToolDefs`.

## Wiring (Spawner ↔ apirun)

Spawner-side (`be/internal/spawner/`):

- `Config` carries `Provider`, `AgentSvc`, `APICredentialRepo`, `FindingsSvc`, `ProjectFindingsSvc`, `AgentSvcReal`, `WorkflowSvc`, `ToolDefRepo`. Set by orchestrator at workflow start.
- `prepareSpawn` (api branch) calls `loadAPIHTTPToolDefs` + `apirun.ResolveRegistry` and stuffs results into `prep.apiTools` / `prep.apiHandlers` / `prep.apiToolEnv`.
- `apiBackend.Start` builds an `apirun.Runner` from the Config and runs it in a goroutine. On exit it persists messages and registers stop via `registerAgentStopWithReason(mapFinalStatus(proc.finalStatus))`.
- `procStateAdapter` exposes `SetFinalStatus`, `SetContextLeft`, `SetCallbackLevel` over `*processInfo`.

`mapFinalStatus` translates runner status to (result, reason):
- `PASS` → (pass, implicit)
- `FAIL` → (fail, api_error)
- `CONTINUE` → (continue, api_continue)  → monitorAll relaunches
- `CALLBACK` → (callback, callback)      → finalizePhase reads `callback_level` finding and returns `CallbackError`
- `CANCELLED` → (fail, cancelled)

## Take-Control Rejection

`apiBackend.SupportsTakeControl()` returns `false`. When a take-control request arrives for a running API agent, `monitorAll` broadcasts `EventAgentTakeControlRejected` (`agent.take_control_rejected`) with `session_id`, `agent_type`, `model_id`, and `reason="api_mode_unsupported"`, then breaks out of the take-control branch — the agent is not killed and continues running normally. The HTTP take-control handlers (ticket-scoped and project-scoped) also perform an early check via `isAPISession()` and return HTTP 409 `api_mode_unsupported` before dispatching to the orchestrator, so the spawner-side broadcast is a belt-and-suspenders fallback for any path that bypasses the HTTP layer.

## Low-Context Behavior

When the per-turn context percentage drops below `restart_threshold`, `monitorAll` sets `proc.lowContextSaving = true` and calls `initiateContextSave`. For API agents, `context_save.go` forces `useAgentSave = true` regardless of the `context_save_via_agent` global setting — the resume path is Claude-CLI-only and cannot be used for an in-process runner. `apiBackend.Kill` cancels the runner's stored context; the `runner.Run` goroutine returns `CANCELLED`, persists messages, and closes `proc.doneCh`. `initiateContextSave` then detects `CANCELLED`, spawns a context-saver agent to summarize the killed agent's message history and write the summary to `to_resume` findings, then calls `relaunchForContinuation` — which sets `finalStatus = CONTINUE` and triggers `monitorAll` to launch a fresh `apirun.Runner` with `${PREVIOUS_DATA}` populated from `to_resume`.

The saver itself can run via the in-process runner when the killed agent is API-mode: `spawnContextSaver` calls `GetForBackend("context-saver", "api")` and, if a `context-saver-api` system agent definition exists, the ephemeral child spawner is configured with `ExecutionMode="api"` and the appropriate `Tools` CSV (e.g., `findings_add`). All API-mode dependencies (`Provider`, `AgentSvc`, `FindingsSvc`, etc.) are forwarded from the parent spawner so the saver's builtin tools work correctly. When no api variant is found, the spawner falls back to the CLI context-saver (structured warn logged).

## Stall Detection

API agents fully participate in stall detection because the `runner.go` + `sink.go` path calls `Spawner.TrackMessage` on every text delta and tool-use event, updating `proc.lastMessageTime` identically to CLI agents. `checkStall` in `stall_restart.go` polls `lastMessageTime` on each `monitorAll` tick and fires `handleStallRestart` when `stallStartTimeout` or `stallRunningTimeout` is exceeded. The stall handler broadcasts `agent.stall_restart`, calls `apiBackend.Kill` (cancels runner ctx), increments `stallRestartCount` (cap: 15), sets `finalStatus = CONTINUE`, and calls `relaunchForContinuation` after a 15s delay. No context save is attempted — the agent is frozen. Instant-stall logic was removed in migration 000061; only `stall_restart.go` remains and it is CLI-agnostic.
