# Spawner Reference

Deep mechanics for this package. The auto-loaded map lives in [CLAUDE.md](CLAUDE.md).

### Codex app-server backend

`codexAppServerBackend` (`codex_appserver_backend.go`) drives `codex app-server` over newline-delimited JSON-RPC stdio (`codex_appserver_client.go`), spawned with `--disable` flags blocking native delegation (`appServerArgs()`) — codex 0.133 emits no PTY hooks (openai/codex#21639) and no rollout JSONL, so app-server is the only structured channel. Events map to the standard `Sink` via `dispatchAppServerEvent` (`codex_appserver_events.go`): agentMessage→text, command/webSearch→tool, mcpToolCall→invoke+result, `thread/tokenUsage`→`context_left`, turn lifecycle→heartbeat, typed rate-limit. Completion stays socket/DB-driven; idle/nudge re-issues a `turn/start` with the `finish-reminder`. `SupportsResume()`/`SupportsTakeControl()` are both false. `CodexAdapter` is still used for model mapping + `ClassifyExit`; its PTY methods are unused. `appServerArgs()` also passes `-c project_doc_fallback_filenames=["AGENTS.md","CLAUDE.md"]` (`codex_project_doc.go`) so a project whose repo has no `AGENTS.md` still gets its root `CLAUDE.md` loaded as the codex project doc (first existing name wins per directory, so nrflo's own `AGENTS.md`->`CLAUDE.md` symlink loads once, not twice — and for this repo the flag is a no-op). This is a CLI override rather than a config.toml key because a key appended after a table header is silently absorbed by that table, and a duplicate root key is a hard parse error against the user's own `~/.codex/config.toml`.

**Nested package `CLAUDE.md` files do NOT reach codex workers.** Codex's project-doc chain only walks *ancestors* of cwd, and every nrflo spawn sets cwd = `Config.ProjectRoot` (`cmd.Dir = workDir`), so the chain is always exactly one directory: the repo root. `be/internal/*/CLAUDE.md` and friends are invisible to a spawned codex agent — it must read them with its own tools (the root doc is the map and points at them). Codex also does not expand `@`-imports in project docs. Closing that gap needs a different mechanism (e.g. injecting the doc tree into the prompt the spawner builds), not this flag; `codex_project_doc_cli_test.go` pins both the positive and the negative.

### Console Engine

`ConsoleEngine` (`console_engine.go`) is the human-session counterpart to `ExecutionBackend`: `GetConsoleEngine(name, sink)` is the one provider-name switch (mirrors `GetCLIAdapter`/`console.GetDriver`), returning a `codexEngine` (`console_engine_codex.go`) today. It takes a `Sink` + a `sessionID`/`EngineSpec` — no `*Spawner`, no `*processInfo`, no DB pool — which is what makes "exempt from autonomous policies" structural rather than a flag: the nudge loop (`codex_appserver_idle.go`), stall detection, and restart caps all live on `processInfo`/`monitorAll` and are simply unreachable from an object that never holds one.

**Normalized event set.** `EngineEvent.Type` is one of `text_delta`, `text`, `thinking`, `tool_invoke`, `tool_result`, `approval_request`, `turn_started`, `turn_completed`, `token_usage`, `error`. These are provider-agnostic; `codexEngine` produces them via `dispatchAppServerEvent`'s `emit EventEmitter` parameter (`codex_appserver_events.go` / `codex_appserver_events_engine.go`), the same mapper the autonomous `codexAppServerBackend` uses.

**Nil-emitter contract.** `dispatchAppServerEvent` and its helpers (`dispatchCompletedItem`, `emitMcpToolCall`, `dispatchTokenUsage`) take `emit EventEmitter`; every emit call goes through `EventEmitter.emit`, which is a no-op when the emitter is nil. `codexAppServerBackend` always passes `nil`, so the autonomous spawn path's `Sink` calls, control-signal (`appServerSignal`) derivation, and heartbeat bumps are byte-for-byte unchanged — the only new work on that path is a nil check per event. Deltas (`item/agentMessage/delta`, `item/reasoning/textDelta`, `item/reasoning/summaryTextDelta`) are live-only: they bump the heartbeat and emit an `EngineEvent` but never call `RecordHookMessage` — the canonical text/thinking is persisted once, on `item/completed`.

**Completed-item field shapes.** `item/completed` carries a *ThreadItem*, and `appServerItem.Content`/`.Summary` are `json.RawMessage` because the same field NAME holds different shapes per item type: a `reasoning` item's `content`/`summary` are arrays of plain strings (`ReasoningThreadItem`, both protocol generations — the `{type,text}`-block shape belongs to the raw `ReasoningItem`, which `item/completed` never carries), while a `userMessage`'s `content` is an array of input blocks. Typing either strictly makes `json.Unmarshal` fail for the other type, and `dispatchCompletedItem`'s error guard then silently drops the whole item — so decoding stays per-type (`reasoningText`), not on the shared struct.

**Events channel + Stop.** `Events()` is a 256-buffered channel and `codexEngine.emit` selects on a `stopping` channel that `Stop` closes *before* it waits on `loopDone`. Without that, a consumer in the natural `for ev := range eng.Events() { … eng.Stop() }` shape — which stops draining the instant it calls `Stop` — parks `runLoop` in a blocking send on a full buffer, so it never re-enters its select to observe `ctx.Done` and `Stop` hangs forever. `runLoop` also clears `turnActive` on exit, so a connection dropped mid-turn cannot pin the engine into permanently rejecting `SendUserTurn` with `ErrTurnActive`.

**Two approval protocol generations, different vocabularies** (validated against `codex app-server generate-json-schema`, codex-cli 0.144.1; see `console_engine_codex_approval.go`'s `approvalDecisionWire`):

| Generation | Methods | approve | approve_for_session | deny | abort |
|---|---|---|---|---|---|
| v2 | `item/commandExecution/requestApproval`, `item/fileChange/requestApproval` | `accept` | `acceptForSession` | `decline` | `cancel` |
| legacy | `execCommandApproval`, `applyPatchApproval` | `approved` | `approved_for_session` | `denied` | `abort` |

`decline`/`denied` means the command is not executed but the turn continues; `cancel`/`abort` also interrupts the turn. `serverRequest/resolved` is a *notification* (no id) meaning the server resolved a pending request elsewhere (or it timed out) — `codexEngine.runLoop` intercepts it ahead of `dispatchAppServerEvent` and drops the matching pending entry without replying. `ReplyApproval` drops a pending entry only *after* the reply is written: `ApprovalDecision` is a bare string, so an unmappable value (or a transport failure) must leave the id retryable rather than consuming it and leaving codex blocked on that JSON-RPC id forever.

**Why `item/permissions/requestApproval` is not decision-shaped.** Its response shape is `{permissions, scope, strictAutoReview}`, not `{decision: ...}` — it configures a permission grant, it doesn't approve/deny one action. `codexEngine.onServerRequest` does not special-case it; like every other unhandled server request (`item/tool/requestUserInput`, `mcpServer/elicitation/request`, `item/tool/call`, `account/chatgptAuthTokens/refresh`, `attestation/generate`, ...) it falls through to `client.replyError`, so codex is never left blocked on a request the engine does not implement. The autonomous backend's blanket `{"decision":"approved"}` auto-reply (`codex_appserver_backend.go`, only valid for the legacy pair and unreachable under `approvalPolicy=never`) is deliberately not reused here.

**Delegation blocking, extended to console.** `appServerArgs()`'s `--disable multi_agent --disable multi_agent_v2 --disable enable_fanout` (cc96eed6) stays on for `codexEngine` even though it launches from a human console session, not a managed spawn: an app-server-spawned child is invisible to nrflo either way, so the rationale that motivated the flag for managed sessions applies unchanged here. This is the one place `codexEngine` deliberately diverges from `console.ConsoleDriver` (`console/driver_codex.go`), which passes no `--disable` flags at all — that driver launches the codex TUI directly at a real terminal, not through the app-server protocol, so the delegation-visibility problem doesn't arise the same way.

## Interactive CLI Backend

`cliInteractiveBackend` (`backend_interactive.go`) spawns CLI agents in a PTY. Per-adapter divergence lives entirely in the adapter file — `backend_interactive.go` has no name-checks. Key `CLIAdapter` methods for interactive use (see `cli_adapter.go`):

- `BuildInteractiveCommand(opts)` — PTY-friendly command without batch flags. When `opts.ResumeSessionID` is set, appends `--resume <id>` (Claude) or the equivalent resume argument (Codex).
- `PrepareInteractive(opts)` — returns `InteractiveExtras` (Codex: per-session CODEX_HOME profile dir).
- `DeliversPromptInline()` — true for Codex (argv positional); false for Claude (PTY stdin write after readiness delay).
- `NeedsTerminalQueryReplies()` — true for Codex (TUI sends terminal capability probes during init).
- `BumpsOnPTYBytes()` — opt-in heartbeat via PTY bytes. Claude returns false (hooks drive the heartbeat). The codex PTY adapter is unused — codex runs on the app-server backend.

`writeCodexProfileForSession` (`cli_adapter_codex_profile.go`) writes the per-session `CODEX_HOME` (auth + a workdir `trust_level="trusted"` entry, without which codex 0.133 blocks on a trust dialog); called by the app-server backend's `Start`.

### Settings Merge (Claude interactive)

`BuildInteractiveSettingsJSON` (`hooks_settings.go`) returns `--settings` JSON with `hooks` (PreToolUse/PostToolUse → `nrflo agent record-event`) and `statusLine`. `mergeInteractiveSettings` deep-merges safety JSON + hooks JSON, concatenating hook arrays on key conflict so `statusLine` survives.

## Rate-Limit Restart

`cli_interactive`: a non-zero exit matching a rate-limit pattern (adapter `ClassifyExit`) triggers `handleRateLimitRetry` (`rate_limit_restart.go`) — broadcasts `agent.rate_limited`, registers `result=continue/reason=rate_limit`, persists `rate_limit_until_ts`, sets `finalStatus=CONTINUE`. `waitForRateLimitRetry` sleeps exponential backoff (`min(InitialBackoff·2^(n-1), MaxWait)`), or waits for a known subscription reset via `resetAwareDelay` (`rate_limit_config.go`, +30s, ≤8h). `rateLimitRetryCount` is separate from `failRestartCount` and carries across relaunches.

In-band: a 529 the Claude CLI prints as text without exiting is caught on idle by `handleInBandRateLimit` (`inband_rate_limit.go`) — same retry, relaunch uses `--fallback-model`.

`api` agents: `apirun.classifyProviderError` returns `RetryClassRateLimit`; `apiBackend.Start` runs the same dance. `rateLimitConfig` loads for both lanes in `prepareSpawn`.

Config keys (project > global, via `pool.GetProjectConfig`/`GetConfig`): `rate_limit_enabled` (default `true`), `rate_limit_initial_backoff_sec` (`60`), `rate_limit_max_wait_sec` (`3600`), `<adapter>_limit_patterns`/`<adapter>_error_patterns` (extra comma-separated patterns).

## Agent Env Vars

| Variable | Purpose |
|----------|---------|
| `NRFLO_PROJECT` | Project ID |
| `NRF_WORKFLOW_INSTANCE_ID` | Workflow instance UUID |
| `NRF_SESSION_ID` | Agent session UUID |
| `NRFLO_AGENT_TOKEN` | Per-session bearer token (`id.MintToken()`) |
| `NRF_SPAWNED` | Set to `1` |
| `NRF_CONTEXT_THRESHOLD` | Context usage threshold % |
| `NRF_MAX_CONTEXT` | Max context window size in tokens |
| `NRF_ARTIFACTS_DIR` | Absolute path to the pre-materialized artifact stage dir (`$NRFLO_HOME/projects/{projectID}/artifacts/{wfiID}/`) |
| `NRF_EXTERNAL_ID` | `external_id` from the workflow instance ("" if unset) |
| `NRF_EXTERNAL_CONTEXT` | `external_context` from the workflow instance ("" if unset) |
| *(per-project vars)* | `Config.ProjectEnv` entries appended last (last-wins) |

## Observer Agent

Entry point: `spawn_observer.go:23`. Called by `ObserverService.Launch` (service/observer.go) — bypasses orchestrator, layer, and phase; no `workflow_instances` row is created. The session row uses `agent_type=_observer`, `phase=observer`, `kind=observer`, and `observer_scope` set to the requested scope. Env vars injected in addition to the standard agent envelope: `NRF_OBSERVER=1`, `NRF_OBSERVER_SCOPE`, `NRF_PROJECT_ID`, and (when scope=workflow) `NRF_WORKFLOW_ID`. Terminates via the standard `CompleteInteractive` / `KillInteractive` interactive-wait path.
