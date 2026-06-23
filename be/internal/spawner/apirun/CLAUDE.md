# apirun Package

> **Note:** Only reachable when the `api_mode_enabled` global setting is `true`. When the setting is off, `prepareSpawn` returns `api_mode_disabled` before constructing a Runner.

In-process tool-use loop for API-mode agents. Files: `runner.go` (turn loop), `interfaces.go` (MessageSink/ProcState/AgentSvc/ErrorRecorder surfaces), `tool.go` (ToolHandler/TerminalSignal/Registry, plus `ToolEnv.DispatchRepo`), `registry.go` (ResolveRegistry + MergeBaseline), `sink.go` (provider events → UI message rows), `errors.go` (error classification), `provider/` (Anthropic and OpenAI streaming impls + mock), `tools_builtin/` (builtin handlers), `tools_python/` (python_scripts kind=tool handler).

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

Builtin tool handlers registered in `tools_builtin/builtins.go`; the map literal there is the canonical list.

`read_document` (`tools_builtin/read_document.go`) materializes a named input artifact and returns its bytes as an image/document content block so the model can read it natively (OCR scanned PDFs, photos). PDF → document block, PNG/JPEG → image block; other types return a text error. Capped at 32 MiB. It implements `MediaToolHandler` (see below).

`consult` (`tools_builtin/consult.go`) synchronously spawns a named consultant agent via `apirun.ConsultantSpawner` and returns the `_consult_answer` finding inline; see [doc/api.md § Consultants](../../../../doc/api.md#consultants) for authoring requirements.

## Multimodal Tool Results

`provider.ContentBlock.OutputMedia []MediaBlock` carries image/document payloads on a `tool_result`. A handler opts in by implementing `apirun.MediaToolHandler` (`InvokeMedia` returns `(text, []MediaBlock, isError, err)`); the runner prefers it over `Invoke` via a type assertion and threads the media into the tool_result. `provider/anthropic/translate.go:translateMediaBlock` maps each `MediaBlock` to the SDK `ToolResultBlockParamContentUnion` (`OfImage` base64 / `OfDocument` base64 PDF). Image media types: jpeg/png/gif/webp; document: application/pdf only.

## Python Tool Handler

`tools_python.New(row, pythonPath, sdkDir, projectEnv)` returns a handler for a `python_scripts` row with `kind=tool`. Each Invoke compiles the JSON schema once (Draft 2020), validates input, writes the script to a temp `.py` (`FilePath` preferred over `Code` when absolute and `.py`), and execs `pythonPath` with input on stdin. Env mirrors `prepareScriptSpawn`: inherits the server env (`os.Environ()` minus `CLAUDECODE`, so the SDK socket resolves via `NRFLO_SOCKET`/`NRFLO_HOME`/`HOME`), then sets `NRFLO_PROJECT`/`NRF_SESSION_ID`/`NRF_WORKFLOW_INSTANCE_ID`/`NRF_TRX`/`NRF_SPAWNED=1`/`NRF_EXTERNAL_ID`/`NRF_EXTERNAL_CONTEXT` (external refs present-but-empty when unset) and `NRFLO_SDK_DIR` (so tool scripts can `import nrflo_sdk`; skipped when `sdkDir` empty), then `projectEnv` (last-wins). Timeout from `row.TimeoutSec` (default 30s); non-zero exit surfaces stderr; stdout capped at 16 KB. Schema/timeout/exit failures return `isError=true` with no Go error. Each Invoke inserts a `tool_dispatches` row and broadcasts `ws.EventToolDispatched`.

## Per-Agent Registry Resolution

`ResolveRegistry(toolsCSV, builtins, pythonHandlers)` composes builtins → python tools. Glob matching: `""` = empty registry; `"*"` = all; `"findings_*"` = prefix glob (note underscore — `findings.*` matches nothing). No match → spawn fails with `no tools matched`. Name collision (python vs builtin) → spawn fails with `collides with builtin`.

`MergeBaseline(specs, handlers, builtins, names)` force-adds missing baseline tools (the `agent_*` lifecycle group plus `findings_add`, `tools_builtin.BaselineToolNames()`). Socket-completion spawns (cli_interactive/codex/api-via-cli) pass `forceBaseline=true` to `buildAPIRegistry` so a restrictive tools CSV can never strip an agent's ability to signal completion or record findings; pure in-process api agents leave it off (they auto-PASS on `end_turn`).

## Wiring

The concrete provider is selected per-agent from the agent's `api_models` row (`provider` column); credentials are resolved per-provider — Anthropic uses OAuth/API-key (`provider/anthropic/credentials.go`), OpenAI uses API-key only (`provider/openai/credentials.go`).

`prepareSpawn` (api branch) calls `loadProjectPythonTools` + `apirun.ResolveRegistry` → `prep.apiTools/apiHandlers`. `apiBackend.Start` builds an `apirun.Runner` in a goroutine. `mapFinalStatus` maps exit status: PASS→(pass,implicit), FAIL→(fail,api_error), CONTINUE→(continue,api_continue), CALLBACK→(callback,callback), CANCELLED→(fail,cancelled), RATE_LIMITED→(continue,rate_limit). See `spawner/api_backend.go`.

## Provider Error Classification

`errors.go:classifyProviderError` returns `(status, message, RetryClass)`. Detection uses typed SDK errors — no string matching. `ctx.Err()` takes priority → `CANCELLED`. Anthropic `*sdk.Error`: type-based detection first (`ErrorTypeRateLimitError | ErrorTypeOverloadedError` or `StatusCode ∈ {429, 529}` → `RATE_LIMITED`), 401/403 → `FAIL auth_error`, 5xx → `FAIL provider_error`. OpenAI `*openai.Error` (alias for `apierror.Error`): 429 → `RATE_LIMITED`, 401/403 → `FAIL auth_error`, 5xx → `FAIL provider_error`. `json.SyntaxError`/`UnmarshalTypeError` → `FAIL provider_protocol_error`. Other → `FAIL` + `RetryClassNone`. On `RetryClassRateLimit` the runner skips `ErrorSvc.RecordError` and sets `RATE_LIMITED`; the api_backend goroutine then performs the rate-limit retry dance (see spawner [Rate-Limit Restart](../CLAUDE.md#rate-limit-restart)) gated on `rateLimitConfig.Enabled && rateLimitTotalWait < MaxWait`.

## Low-Context Behavior

`context_save.go` forces `useAgentSave=true` for API agents (resume path is Claude-CLI-only). `apiBackend.Kill` cancels runner ctx → saver agent summarizes history → `relaunchForContinuation` with `${PREVIOUS_DATA}`.

## Extended Thinking (Anthropic)

The Anthropic provider decodes `thinking` and `redacted_thinking` content blocks from the stream and assembles them into `provider.ContentBlock` values with `Type="thinking"` (fields: `Text`, `Signature`) or `Type="redacted_thinking"` (field: `Data`). These blocks are appended verbatim to `resp.Content` and replayed in subsequent assistant turns — this replay is always enabled when thinking is on and is never gated.

Thinking is opt-in via the per-agent `ReasoningEffort` from the `api_models` row (empty → off). `translate.go:translateRequest` picks the shape by model family (`model.go:is46Plus`): **4.6+ models** (Opus 4.6/4.7/4.8, Sonnet 4.6) get adaptive thinking (`ThinkingConfigAdaptiveParam`) + `OutputConfig.Effort` (`effortParam`: low/medium/high/xhigh, unknown→medium) — the `enabled`+`budget_tokens` shape 400s on Opus 4.7/4.8. **Haiku 4.5 and older** keep `ThinkingConfigParamOfEnabled(budget)` (`thinkingBudget`: `low`→4096, `medium`→8192, `high`→16384, `xhigh`→24576) with `max_tokens` raised to `budget+4096`; they have no adaptive mode and reject effort. The model id is also `[1m]`-stripped (`stripContextSuffix`) before the request — the bracket marker is a CLI convention that 404s on the API and only selects the context window. Temperature is left unset.

Display gate: thinking deltas are emitted as `category="thinking"` agent-log rows only when `capture_thinking_enabled` is true (project > global > default false). `Config.CaptureThinking` carries the resolved flag from `spawner_prepare.go` → `apirun.Config` → `newRunnerSink`. The separate `thinkBuf` in `runnerSink` ensures thinking content is never mixed into `"text"` rows and is flushed before text/tool-use within each turn.

## Prompt Caching (Anthropic)

The runner re-sends the full transcript every turn, so without caching a long tool loop re-bills the whole conversation each turn. `apiBackend.Start` sets `Config.CacheBreakpoints` to `{system, message}`; the runner forwards them on every `provider.Request`. Anthropic placement + the 4-slot / 20-block-lookback handling live in `provider/anthropic/cache.go`: the system marker also caches the tool definitions (tools render first), and `CacheTargetMessage` slides a marker onto the latest message each turn, adding intermediate markers when a turn appends more blocks than the lookback window. Markers MUST carry a non-zero TTL (`ephemeralCacheControl()`) — a zero-valued `CacheControlEphemeralParam{}` is dropped by the SDK's `omitzero` tag. OpenAI ignores breakpoints. Effectiveness is observable per turn: `Runner.updateContext` logs an `apirun turn usage` line (`input`/`cache_read`/`cache_creation`/`output`, `cache=hit|miss`, `cache_hit_pct`) — the only place the cache split surfaces, since it is otherwise summed into the context-left %.

## Stall Detection

`runner.go`/`sink.go` call `TrackMessage` on each text/tool-use event, identical to CLI agents. Stall detection in `stall_restart.go`; cap 15 restarts.

Run `make test-pkg PKG=spawner/apirun`.
