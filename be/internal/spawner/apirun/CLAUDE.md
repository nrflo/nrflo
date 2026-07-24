# apirun Package

> **Note:** Only reachable when the `api_mode_enabled` global setting is `true`. When the setting is off, `prepareSpawn` returns `api_mode_disabled` before constructing a Runner.

In-process tool-use loop for API-mode agents. Files: `runner.go` (Run + Config), `runner_loop.go` (shared `runTurns`/`dispatchTools` loop), `conversation.go` (`Conversation`/`StreamHook`, multi-turn), `interfaces.go` (MessageSink/ProcState/AgentSvc/ErrorRecorder surfaces), `tool.go` (ToolHandler/TerminalSignal/Registry, plus `ToolEnv.DispatchRepo`), `registry.go` (ResolveRegistry + MergeBaseline), `sink.go` (provider events → UI message rows), `errors.go` (error classification), `provider/` (Anthropic and OpenAI streaming impls + mock), `tools_builtin/` (builtin handlers), `tools_python/` (python_scripts kind=tool handler).

## Conversation (multi-turn)

`Conversation` (`conversation.go`) preserves `[]provider.Message` history across `SendTurn` calls instead of Run's single-shot session, auto-compacting near the context limit and sharing the same `runTurns` loop as Run. Mechanics (compaction thresholds, `ResumeTurn`, `Config.Stream` delta/itemID semantics): [REFERENCE.md](REFERENCE.md#conversation-multi-turn).

## Tool Dispatch Flow

- `Provider.Run` returns `StopReason`: `end_turn` → `SetFinalStatus("PASS")`; `max_tokens` → fail with system message.
- On `tool_use`: tool_use blocks dispatch concurrently (cap 4, `maxParallelToolDispatch`), results assembled in original block order; the first `TerminalSignal` in block order sets `proc.finalStatus` and returns early.
- Non-terminal results appended as tool_result messages; loop continues for next turn.
- Blob quarantine: successful text results over `tool_result_offload_threshold_bytes` (default 8KB, project>global config; `tool_result_offload_enabled` gate) are stored as a content-addressed `toolres_*` artifact and replaced inline by a head+tail excerpt with an `artifact_get` pointer (`MaybeOffloadToolResult`, `tool_offload.go`). Applies to in-process api dispatch and the bridge path (`spawner.DispatchTool`); exempt: `artifact_get`, `web_fetch`/`web_search`, `read_document`, `agent_*`; sessions without artifact scope (nil svc / no workflow instance) pass through unchanged.
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

Builtin tool handlers registered in `tools_builtin/builtins.go`; the map literal there is the canonical list. `tools_builtin.FSTools()` (`fs*.go`) offers the native fs/shell tools toward Claude Code parity (read_file/edit_file/write_file/glob/grep/bash/bash_output/kill_shell), workdir-jailed via `resolveFSPath`; merged only when `api_native_tools_enabled` is on, except cli_interactive claude defs resolving `native_tools=="none"`. Per-session read-set + background-shell state lives on `ToolEnv.FS`; `bash` also runs through `ToolEnv.SafetyCheck`, a script-only gate (not a permission system). Mechanics: [REFERENCE.md](REFERENCE.md#native-fs-tools).

`read_document` (`tools_builtin/read_document.go`) inlines a named input artifact as media (OCR): PDF → document block, PNG/JPEG → image block, others → text error; 32 MiB cap. Implements `MediaToolHandler` (see below).

`consult` (`tools_builtin/consult.go`) synchronously spawns a named consultant agent via `apirun.ConsultantSpawner` and returns the `_consult_answer` finding inline; see [doc/api.md § Consultants](../../../../doc/api.md#consultants) for authoring requirements.

`delegate` / `get_delegation` (`tools_builtin/delegate.go`, `get_delegation.go`) spawn tier-resolved (`extractor`/`executor`) workers downward via `apirun.Delegator`, async-with-poll like the sub-workflow builtins; mechanics: [REFERENCE.md](REFERENCE.md#delegate--get_delegation-builtins).

`findings_add_from_file` (`tools_builtin/findings_from_file.go`) stores a workdir-jailed file's content as a finding via the normal `FindingsService.Add` path (256KB cap, rejects non-UTF8), returning `{key,bytes,sha256}` so agents can persist existing text without re-streaming it through the model; it is force-baseline like `findings_add`.

`web_search` / `web_fetch` (`tools_builtin/web_search.go`, `web_fetch.go`) call the provider-agnostic `tools_web` layer (SearXNG search + direct fetch by default; provider selected via `web_search_provider`/`web_fetch_provider` config, egress via `WEB_PROXY_URL`). `web_fetch` SSRF-guards every URL via a pinning dialer (`tools_web/urlguard.go`, `egress_dial.go`), returns an excerpt inline and offloads full markdown to an artifact; blocked/failed fetches surface as `ok:false` rather than failing the turn. These are backend-agnostic (available to cli/codex agents over the MCP bridge too).

`run_subworkflow` / `get_subworkflow` / `dynamic_workflow` / `revise_plan` / `approve_plan` start/poll/drive child workflows via `ToolEnv.Subworkflows` — async-with-poll, bounded `wait_sec` ≤240 (heartbeated). Server-side guards + plan statuses: [orchestrator/REFERENCE.md](../../orchestrator/REFERENCE.md#sub-workflow-runner); payload shapes + depth-based tool strip: [REFERENCE.md](REFERENCE.md#sub-workflow--dynamic-workflow-builtins). Feature-gated by `subworkflow_tools_enabled` (default on).

## Multimodal Tool Results

`provider.ContentBlock.OutputMedia []MediaBlock` carries image/document payloads on a `tool_result`. A handler opts in by implementing `apirun.MediaToolHandler` (`InvokeMedia` returns `(text, []MediaBlock, isError, err)`); the runner prefers it over `Invoke` via a type assertion and threads the media into the tool_result. Both providers render it — Anthropic: `provider/anthropic/translate.go:translateMediaBlock` maps each block into the tool_result content (image/document base64); OpenAI: `provider/openai/translate_media.go` appends a user-role message after the `function_call_output` (Responses API forbids media on tool outputs) with `input_image` / `input_file` (`filename` + `file_data` data URL) parts, both detail=high. Image media types: jpeg/png/gif/webp; document: application/pdf only.

## Python Tool Handler

`tools_python.New(row, pythonPath, sdkDir, projectEnv)` returns a handler for a `python_scripts` row with `kind=tool`; mechanics (schema validation, env, timeout, offload): [REFERENCE.md](REFERENCE.md#python-tool-handler).

## Per-Agent Registry Resolution

`ResolveRegistry(toolsCSV, builtins, pythonHandlers)` composes builtins → python tools. Glob matching: `""` = empty registry; `"*"` = all; `"findings_*"` = prefix glob (note underscore — `findings.*` matches nothing). No match → spawn fails with `no tools matched`. Name collision (python vs builtin) → spawn fails with `collides with builtin`.

`MergeBaseline(specs, handlers, builtins, names)` force-adds missing baseline tools (the `agent_*` lifecycle group plus `findings_add`, `tools_builtin.BaselineToolNames()`). Socket-completion spawns (cli_interactive/codex/api-via-cli) pass `forceBaseline=true` to `buildAPIRegistry` so a restrictive tools CSV can never strip an agent's ability to signal completion or record findings; pure in-process api agents leave it off (they auto-PASS on `end_turn`).

## Wiring

The concrete provider is selected per-agent from the unified `models` row (`provider` column); credentials are resolved per-provider — Anthropic uses OAuth/API-key (`provider/anthropic/credentials.go`), OpenAI uses API-key only (`provider/openai/credentials.go`). `OPENAI_BASE_URL` resolves per-project → server env, so one project can route through an OpenAI-compatible proxy without affecting others. `provider/openrouter` is a third, api-mode-only provider: a thin wrapper constructing `openai.New` with the OpenRouter base URL, ladder-resolving its own `OPENROUTER_API_KEY`/`OPENROUTER_BASE_URL`. `provider/custom` mirrors it for `custom_providers` registry rows (BYO OpenAI-compatible servers), picking `provider/openai` (Responses API), the chat-completions-only `provider/openaichat`, or `provider/ollamanative` per the row's `api_wire`; credentials come straight from the DB row, no env-ladder. `provider/ollamanative` speaks Ollama's native NDJSON `POST /api/chat` over plain `net/http` (no openai-go SDK) so it can send `think:false`/`true`, mapped from the row's `reasoning_effort` (`none`/empty → `false`, any real level → `true`) — the only wire able to disable hybrid-thinking models.

`prepareSpawn` (api branch) calls `loadProjectPythonTools` + `apirun.ResolveRegistry` → `prep.apiTools/apiHandlers`. `apiBackend.Start` builds an `apirun.Runner` in a goroutine. `mapFinalStatus` maps exit status: PASS→(pass,implicit), FAIL→(fail,api_error), CONTINUE→(continue,api_continue), CALLBACK→(callback,callback), CANCELLED→(fail,cancelled), RATE_LIMITED→(continue,rate_limit). See `spawner/api_backend.go`.

The api-mode system prompt renders from the `api-system-prompt` injectable (fallback to the `defaultAPISystemPrompt` constant), with `system-prompt-suffix` appended for autonomous workers.

## Provider Error Classification

`errors.go:classifyProviderError` maps typed SDK errors (never string matching) to `(status, message, RetryClass)`: rate-limit/overload → `RATE_LIMITED` (spawner retry dance), 401/403 → auth fail, 5xx → provider fail, JSON decode → protocol fail; `ctx.Err()` wins as `CANCELLED`. Full matrix: [REFERENCE.md](REFERENCE.md#provider-error-classification).

## Low-Context Behavior

`context_save.go` forces `useAgentSave=true` for API agents (resume path is Claude-CLI-only). `apiBackend.Kill` cancels runner ctx → saver agent summarizes history → `relaunchForContinuation` with `${PREVIOUS_DATA}`.

## Extended Thinking (Anthropic)

Opt-in via the unified model row's API reasoning effort. Adaptive 1M models (Fable 5, Mythos 5, Sonnet 5, Opus 4.6/4.7/4.8) get adaptive thinking + `OutputConfig.Effort`; Haiku 4.5 and older keep budgeted `thinking` blocks. Thinking/redacted blocks are replayed in later turns; `capture_thinking_enabled` gates display-only rows. Shapes, budgets, and the `[1m]` strip: [REFERENCE.md](REFERENCE.md#extended-thinking-anthropic).

## Prompt Caching (Anthropic)

`Config.CacheBreakpoints = {system, message}` on every request; placement + 4-slot/lookback handling in `provider/anthropic/cache.go`; markers MUST carry a non-zero TTL. Per-turn effectiveness is logged as `apirun turn usage`. Details: [REFERENCE.md](REFERENCE.md#prompt-caching-anthropic).

## Stall Detection

`runner.go`/`sink.go` call `TrackMessage` on each text/tool-use event, identical to CLI agents. Stall detection in `stall_restart.go`; cap 15 restarts.

`Config.Observer` (`LedgerObserver`, nil-safe) receives every newly appended `provider.ContentBlock` plus each turn's `provider.Usage`, feeding the spawner's external context ledger — see [spawner/CLAUDE.md](../CLAUDE.md#context-ledger). `Config.Watcher` (`ContextWatcher`, nil-safe) is consulted alongside it at each compaction checkpoint and drives selective GC (`runner_compact_selective.go`) in place of the uniform fallback — see [spawner/CLAUDE.md](../CLAUDE.md#context-watcher).

Run `make test-pkg PKG=spawner/apirun`.
