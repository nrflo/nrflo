# apirun Package

> **Note:** Only reachable when the `api_mode_enabled` global setting is `true`. When the setting is off, `prepareSpawn` returns `api_mode_disabled` before constructing a Runner.

In-process tool-use loop for API-mode agents. Files: `runner.go` (turn loop), `interfaces.go` (MessageSink/ProcState/AgentSvc/ErrorRecorder surfaces), `tool.go` (ToolHandler/TerminalSignal/Registry, plus `ToolEnv.DispatchRepo`), `registry.go` (ResolveRegistry), `secret_resolver.go` (secret deref), `sink.go` (event coalescing), `errors.go` (error classification), `provider/` (Anthropic streaming impl + mock), `tools_builtin/` (builtin handlers), `tools_http/` (HTTP tool handler), `tools_python/` (python_scripts kind=tool handler).

## Tool Dispatch Flow

- `Provider.Run` returns `StopReason`: `end_turn` → `SetFinalStatus("PASS")`; `max_tokens` → fail with system message.
- On `tool_use`: `handler.Invoke(ctx, env, block.Input)` per content block; `TerminalSignal` → set `proc.finalStatus` and return early.
- Non-terminal results appended as tool_result messages; loop continues for next turn.

## Terminal Signals

| Signal | Set By | Triggers |
|--------|--------|----------|
| `PASS` | `agent_finished` | spawner: result=pass, reason=finished |
| `FAIL` | `agent_fail` | spawner: result=fail |
| `CONTINUE` | `agent_continue` | `relaunchForContinuation` |
| `CALLBACK` | `agent_callback` | `finalizePhase` reads `callback_level`, returns `CallbackError` |
| `RATE_LIMITED` | `classifyProviderError` → `RetryClassRateLimit` | api_backend rate-limit dance → `relaunchForContinuation` |

Each terminal handler also calls the corresponding `AgentService` method, so DB row + WS broadcast happen identically to CLI agents.

## Builtins

Builtin tool handlers registered in `tools_builtin/builtins.go`; run `grep -n Register tools_builtin/*.go` for the canonical list.

## HTTP Tool Handler

`tools_http.New(client)` returns a factory bound to a shared `http.Client`. Handlers POST `{"tool":<name>,"input":<input>,"context":{...}}` to `def.Endpoint` with timeout (`def.TimeoutSec`, default 30s), auth per `def.AuthMethod` (none/bearer_env/bearer_secret_ref), 5xx retry once, 16 KB body cap. Every Invoke (success and HTTP error) inserts a `tool_dispatches` row via `env.DispatchRepo` and broadcasts `ws.EventToolDispatched`; nil-safe when those fields are unset.

## Python Tool Handler

`tools_python.New(row, pythonPath, projectEnv)` returns a handler for a `python_scripts` row with `kind=tool`. Each Invoke compiles the JSON schema once (Draft 2020), validates input, writes the script to a temp `.py` (`FilePath` preferred over `Code` when absolute and `.py`), and execs `pythonPath` with input on stdin. Env mirrors `prepareScriptSpawn`'s `NRFLO_PROJECT`/`NRF_SESSION_ID`/`NRF_WORKFLOW_INSTANCE_ID`/`NRF_TRX`/`NRF_SPAWNED=1` followed by `projectEnv` (last-wins). Timeout from `row.TimeoutSec` (default 30s); non-zero exit surfaces stderr; stdout capped at 16 KB. Schema/timeout/exit failures return `isError=true` with no Go error. Each Invoke inserts a `tool_dispatches` row and broadcasts `ws.EventToolDispatched`.

## Per-Agent Registry Resolution

`ResolveRegistry(toolsCSV, builtins, pythonHandlers, httpDefs, httpFactory)` composes builtins → python tools → HTTP defs. Glob matching: `""` = empty registry; `"*"` = all; `"findings.*"` = prefix glob. No match → spawn fails with `no tools matched`. Name collision → spawn fails with `collides with` error. Python collides with builtin: error. HTTP collides with builtin or python: error. HTTP scope: `project_id IS NULL OR == agent.project_id` AND `workflow_id IS NULL OR == agent.workflow_name`.

## Wiring

`prepareSpawn` (api branch) calls `loadAPIHTTPToolDefs` + `loadProjectPythonTools` + `apirun.ResolveRegistry` → `prep.apiTools/apiHandlers`. `apiBackend.Start` builds an `apirun.Runner` in a goroutine. `mapFinalStatus` maps exit status: PASS→(pass,implicit), FAIL→(fail,api_error), CONTINUE→(continue,api_continue), CALLBACK→(callback,callback), CANCELLED→(fail,cancelled), RATE_LIMITED→(continue,rate_limit). See `spawner/api_backend.go`.

## Provider Error Classification

`errors.go:classifyProviderError` returns `(status, message, RetryClass)`. Detection uses typed SDK errors only — no string matching. `sdk.Error.Type() == ErrorTypeRateLimitError | ErrorTypeOverloadedError`, or `StatusCode ∈ {429, 529}` → `RATE_LIMITED` + `RetryClassRateLimit`. 401/403 → `FAIL` + `RetryClassError` (auth_error). 5xx → `FAIL` + `RetryClassError`. `json.SyntaxError`/`UnmarshalTypeError` → `FAIL` + `RetryClassError`. Other → `FAIL` + `RetryClassNone`. `ctx.Err()` takes priority → `CANCELLED`. On `RetryClassRateLimit` the runner skips `ErrorSvc.RecordError` and sets `RATE_LIMITED`; the api_backend goroutine then performs the rate-limit retry dance (see spawner [Rate-Limit Restart](../CLAUDE.md#rate-limit-restart)) gated on `rateLimitConfig.Enabled && rateLimitTotalWait < MaxWait`.

## Low-Context Behavior

`context_save.go` forces `useAgentSave=true` for API agents (resume path is Claude-CLI-only). `apiBackend.Kill` cancels runner ctx → saver agent summarizes history → `relaunchForContinuation` with `${PREVIOUS_DATA}`.

## Stall Detection

`runner.go`/`sink.go` call `TrackMessage` on each text/tool-use event, identical to CLI agents. Stall detection in `stall_restart.go`; cap 15 restarts.

Run `make test-pkg PKG=spawner/apirun`.
