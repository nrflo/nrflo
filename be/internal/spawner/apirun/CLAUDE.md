# apirun Package

> **Note:** Only reachable when the `api_mode_enabled` global setting is `true`. When the setting is off, `prepareSpawn` returns `api_mode_disabled` before constructing a Runner.

In-process tool-use loop for API-mode agents. Files: `runner.go` (Run + Config), `runner_loop.go` (shared `runTurns`/`dispatchTools` loop), `conversation.go` (`Conversation`/`StreamHook`, multi-turn), `interfaces.go` (MessageSink/ProcState/AgentSvc/ErrorRecorder surfaces), `tool.go` (ToolHandler/TerminalSignal/Registry, plus `ToolEnv.DispatchRepo`), `registry.go` (ResolveRegistry + MergeBaseline), `sink.go` (provider events → UI message rows), `errors.go` (error classification), `provider/` (Anthropic and OpenAI streaming impls + mock), `tools_builtin/` (builtin handlers), `tools_python/` (python_scripts kind=tool handler).

## Conversation (multi-turn)

`Conversation` (`conversation.go`) preserves `[]provider.Message` history across `SendTurn` calls instead of Run's single-shot session; on `end_turn` it returns the turn's status instead of finalizing the session, and `MaxIterations` applies per `SendTurn`, not per session. When the previous turn left ≤15% of the window, `SendTurn` first auto-compacts (`conversation_compact.go`): one tool-less summarize call replaces the history with a summary+ack pair and emits a `"conversation compacted"` system row (failure is non-fatal). `runTurns` additionally compacts mid-loop at `Config.CompactPct` (`runner_compact.go`, replacement is a single user message keeping `InitialPrompt`); the spawner passes `restartThreshold+5` for autonomous agents so in-process compaction preempts the kill+saver+relaunch dance. `ResumeTurn` re-runs the loop without appending a user message — the api console engine's bounded rate-limit retry path (`console_engine_api.go`). Both share the identical `runTurns` loop (`runner_loop.go`) — no copy. `Config.Stream` (nil for Run) receives raw text/thinking deltas before the sink's chunked buffering, for a live console consumer. Each delta carries the `itemID` of the segment accumulating in the sink's buffer, and that id rotates on every flush, so one id covers exactly one persisted row — a consumer keying its live buffer by id (the FE's delta dedupe) drops it when the row lands instead of concatenating the whole session.

## Tool Dispatch Flow

- `Provider.Run` returns `StopReason`: `end_turn` → `SetFinalStatus("PASS")`; `max_tokens` → fail with system message.
- On `tool_use`: tool_use blocks dispatch concurrently (cap 4, `maxParallelToolDispatch`), results assembled in original block order; the first `TerminalSignal` in block order sets `proc.finalStatus` and returns early.
- Non-terminal results appended as tool_result messages; loop continues for next turn.
- Trace tool spans: the streaming sink emits invoke rows via `MessageSink.TrackToolInvoke` (payload carries `tool_use_id`); after each handler returns, the runner calls `MessageSink.CloseToolSpan` to stamp `ended_at` (spawner side: in-memory pending-buffer stamp, DB fallback — `spawner/output_tool_span.go`).

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

Builtin tool handlers registered in `tools_builtin/builtins.go`; the map literal there is the canonical list. `tools_builtin.FSTools()` (`fs*.go`) is deliberately separate: workdir-jailed `read_file`/`edit_file`/`bash` (symlink-resolved jail via `resolveFSPath`), merged only when the `api_native_tools_enabled` global setting is on — pure in-process api spawns get them via `buildAPIRegistry(includeFS=true)`, console api chats via the engine's approval gate (`spawner/console_engine_api_approval.go`, `FSApprovalRequired` = edit_file/bash).

`read_document` (`tools_builtin/read_document.go`) inlines a named input artifact as media (OCR): PDF → document block, PNG/JPEG → image block, others → text error; 32 MiB cap. Implements `MediaToolHandler` (see below).

`consult` (`tools_builtin/consult.go`) synchronously spawns a named consultant agent via `apirun.ConsultantSpawner` and returns the `_consult_answer` finding inline; see [doc/api.md § Consultants](../../../../doc/api.md#consultants) for authoring requirements.

`web_search` / `web_fetch` (`tools_builtin/web_search.go`, `web_fetch.go`) call the provider-agnostic `tools_web` layer (Exa search + Jina fetch by default; provider selected via `web_search_provider`/`web_fetch_provider` config, API keys via project/server env). `web_fetch` SSRF-guards every URL (`tools_web/urlguard.go`), returns an excerpt inline and offloads full markdown to an artifact; blocked/failed fetches surface as `ok:false` rather than failing the turn. These are backend-agnostic (available to cli/codex agents over the MCP bridge too).

`run_subworkflow` / `get_subworkflow` / `dynamic_workflow` / `revise_plan` / `approve_plan` start/poll/drive child workflows via `ToolEnv.Subworkflows` — async-with-poll, bounded `wait_sec` ≤240 (heartbeated). Server-side guards + plan statuses: [orchestrator/REFERENCE.md](../../orchestrator/REFERENCE.md#sub-workflow-runner); payload shapes + depth-based tool strip: [REFERENCE.md](REFERENCE.md#sub-workflow--dynamic-workflow-builtins). Feature-gated by `subworkflow_tools_enabled` (default on).

## Multimodal Tool Results

`provider.ContentBlock.OutputMedia []MediaBlock` carries image/document payloads on a `tool_result`. A handler opts in by implementing `apirun.MediaToolHandler` (`InvokeMedia` returns `(text, []MediaBlock, isError, err)`); the runner prefers it over `Invoke` via a type assertion and threads the media into the tool_result. Both providers render it — Anthropic: `provider/anthropic/translate.go:translateMediaBlock` maps each block into the tool_result content (image/document base64); OpenAI: `provider/openai/translate_media.go` appends a user-role message after the `function_call_output` (Responses API forbids media on tool outputs) with `input_image` / `input_file` (`filename` + `file_data` data URL) parts, both detail=high. Image media types: jpeg/png/gif/webp; document: application/pdf only.

## Python Tool Handler

`tools_python.New(row, pythonPath, sdkDir, projectEnv)` returns a handler for a `python_scripts` row with `kind=tool`. Each Invoke compiles the JSON schema once (Draft 2020), validates input, writes the script to a temp `.py` (`FilePath` preferred over `Code` when absolute and `.py`), and execs `pythonPath` with input on stdin. Env mirrors `prepareScriptSpawn`: inherits the server env (`os.Environ()` minus `CLAUDECODE`/`CLAUDE_CODE_*`, so the SDK socket resolves via `NRFLO_SOCKET`/`NRFLO_HOME`/`HOME`), then sets `NRFLO_PROJECT`/`NRF_SESSION_ID`/`NRF_WORKFLOW_INSTANCE_ID`/`NRF_TRX`/`NRF_SPAWNED=1`/`NRF_EXTERNAL_ID`/`NRF_EXTERNAL_CONTEXT` (external refs present-but-empty when unset) and `NRFLO_SDK_DIR` (so tool scripts can `import nrflo_sdk`; skipped when `sdkDir` empty), then `projectEnv` (last-wins). Timeout from `row.TimeoutSec` (default 30s); non-zero exit surfaces stderr; stdout capped at 16 KB. Schema/timeout/exit failures return `isError=true` with no Go error. Each Invoke inserts a `tool_dispatches` row and broadcasts `ws.EventToolDispatched`.

## Per-Agent Registry Resolution

`ResolveRegistry(toolsCSV, builtins, pythonHandlers)` composes builtins → python tools. Glob matching: `""` = empty registry; `"*"` = all; `"findings_*"` = prefix glob (note underscore — `findings.*` matches nothing). No match → spawn fails with `no tools matched`. Name collision (python vs builtin) → spawn fails with `collides with builtin`.

`MergeBaseline(specs, handlers, builtins, names)` force-adds missing baseline tools (the `agent_*` lifecycle group plus `findings_add`, `tools_builtin.BaselineToolNames()`). Socket-completion spawns (cli_interactive/codex/api-via-cli) pass `forceBaseline=true` to `buildAPIRegistry` so a restrictive tools CSV can never strip an agent's ability to signal completion or record findings; pure in-process api agents leave it off (they auto-PASS on `end_turn`).

## Wiring

The concrete provider is selected per-agent from the agent's `api_models` row (`provider` column); credentials are resolved per-provider — Anthropic uses OAuth/API-key (`provider/anthropic/credentials.go`), OpenAI uses API-key only (`provider/openai/credentials.go`).

`prepareSpawn` (api branch) calls `loadProjectPythonTools` + `apirun.ResolveRegistry` → `prep.apiTools/apiHandlers`. `apiBackend.Start` builds an `apirun.Runner` in a goroutine. `mapFinalStatus` maps exit status: PASS→(pass,implicit), FAIL→(fail,api_error), CONTINUE→(continue,api_continue), CALLBACK→(callback,callback), CANCELLED→(fail,cancelled), RATE_LIMITED→(continue,rate_limit). See `spawner/api_backend.go`.

## Provider Error Classification

`errors.go:classifyProviderError` maps typed SDK errors (never string matching) to `(status, message, RetryClass)`: rate-limit/overload → `RATE_LIMITED` (spawner retry dance), 401/403 → auth fail, 5xx → provider fail, JSON decode → protocol fail; `ctx.Err()` wins as `CANCELLED`. Full matrix: [REFERENCE.md](REFERENCE.md#provider-error-classification).

## Low-Context Behavior

`context_save.go` forces `useAgentSave=true` for API agents (resume path is Claude-CLI-only). `apiBackend.Kill` cancels runner ctx → saver agent summarizes history → `relaunchForContinuation` with `${PREVIOUS_DATA}`.

## Extended Thinking (Anthropic)

Opt-in via the api_models row's `ReasoningEffort`. 4.6+ models get adaptive thinking + `OutputConfig.Effort`; Haiku 4.5 and older keep budgeted `thinking` blocks. Thinking/redacted blocks are replayed in later turns; `capture_thinking_enabled` gates display-only rows. Shapes, budgets, and the `[1m]` strip: [REFERENCE.md](REFERENCE.md#extended-thinking-anthropic).

## Prompt Caching (Anthropic)

`Config.CacheBreakpoints = {system, message}` on every request; placement + 4-slot/lookback handling in `provider/anthropic/cache.go`; markers MUST carry a non-zero TTL. Per-turn effectiveness is logged as `apirun turn usage`. Details: [REFERENCE.md](REFERENCE.md#prompt-caching-anthropic).

## Stall Detection

`runner.go`/`sink.go` call `TrackMessage` on each text/tool-use event, identical to CLI agents. Stall detection in `stall_restart.go`; cap 15 restarts.

Run `make test-pkg PKG=spawner/apirun`.
