# Spawner Reference

Deep mechanics for this package. The auto-loaded map lives in [CLAUDE.md](CLAUDE.md).

### Codex app-server backend

`codexAppServerBackend` (`codex_appserver_backend.go`) drives `codex app-server` over newline-delimited JSON-RPC stdio (`codex_appserver_client.go`), spawned with `--disable` flags blocking native delegation (`appServerArgs()`) — codex 0.133 emits no PTY hooks (openai/codex#21639) and no rollout JSONL, so app-server is the only structured channel. Events map to the standard `Sink` via `dispatchAppServerEvent` (`codex_appserver_events.go`): agentMessage→text, command/webSearch→tool, mcpToolCall→invoke+result, `thread/tokenUsage`→`context_left`, turn lifecycle→heartbeat, typed rate-limit. Completion stays socket/DB-driven; idle/nudge re-issues a `turn/start` with the `finish-reminder`. `SupportsResume()`/`SupportsTakeControl()` are both false. `CodexAdapter` is still used for model mapping + `ClassifyExit`; its PTY methods are unused. `appServerArgs()` also passes `-c project_doc_fallback_filenames=["AGENTS.md","CLAUDE.md"]` (`codex_project_doc.go`) so a project whose repo has no `AGENTS.md` still gets its root `CLAUDE.md` loaded as the codex project doc (first existing name wins per directory, so nrflo's own `AGENTS.md`->`CLAUDE.md` symlink loads once, not twice — and for this repo the flag is a no-op). This is a CLI override rather than a config.toml key because a key appended after a table header is silently absorbed by that table, and a duplicate root key is a hard parse error against the user's own `~/.codex/config.toml`.

**Nested package `CLAUDE.md` files do NOT reach codex workers.** Codex's project-doc chain only walks *ancestors* of cwd, and every nrflo spawn sets cwd = `Config.ProjectRoot` (`cmd.Dir = workDir`), so the chain is always exactly one directory: the repo root. `be/internal/*/CLAUDE.md` and friends are invisible to a spawned codex agent — it must read them with its own tools (the root doc is the map and points at them). Codex also does not expand `@`-imports in project docs. Closing that gap needs a different mechanism (e.g. injecting the doc tree into the prompt the spawner builds), not this flag; `codex_project_doc_cli_test.go` pins both the positive and the negative.

### Console Engine

`ConsoleEngine` (`console_engine.go`) is the conversation counterpart to `ExecutionBackend`; `GetConsoleEngine` is the one provider-name switch for Codex, Claude, and direct API engines. It takes a `Sink` + `EngineSpec` — no `*Spawner` or `*processInfo` — which makes autonomous nudge, stall, and restart policies structurally unreachable.

**Normalized event set.** `EngineEvent.Type` is one of `text_delta`, `text`, `thinking`, `tool_invoke`, `tool_result`, `approval_request`, `turn_started`, `turn_completed`, `token_usage`, `error`. These are provider-agnostic; `codexEngine` produces them via `dispatchAppServerEvent`'s `emit EventEmitter` parameter (`codex_appserver_events.go` / `codex_appserver_events_engine.go`), the same mapper the autonomous `codexAppServerBackend` uses.

**Nil-emitter contract.** `dispatchAppServerEvent` and its helpers (`dispatchCompletedItem`, `emitMcpToolCall`, `dispatchTokenUsage`) take `emit EventEmitter`; every emit call goes through `EventEmitter.emit`, which is a no-op when the emitter is nil. `codexAppServerBackend` always passes `nil`, so the autonomous spawn path's `Sink` calls, control-signal (`appServerSignal`) derivation, and heartbeat bumps are byte-for-byte unchanged — the only new work on that path is a nil check per event. Deltas (`item/agentMessage/delta`, `item/reasoning/textDelta`, `item/reasoning/summaryTextDelta`) are live-only: they bump the heartbeat and emit an `EngineEvent` but never call `RecordHookMessage` — the canonical text/thinking is persisted once, on `item/completed`.

**Completed-item field shapes.** `item/completed` carries a *ThreadItem*, and `appServerItem.Content`/`.Summary` are `json.RawMessage` because the same field NAME holds different shapes per item type: a `reasoning` item's `content`/`summary` are arrays of plain strings (`ReasoningThreadItem`, both protocol generations — the `{type,text}`-block shape belongs to the raw `ReasoningItem`, which `item/completed` never carries), while a `userMessage`'s `content` is an array of input blocks. Typing either strictly makes `json.Unmarshal` fail for the other type, and `dispatchCompletedItem`'s error guard then silently drops the whole item — so decoding stays per-type (`reasoningText`), not on the shared struct.

**Events channel + Stop.** `Events()` is a 256-buffered channel and `codexEngine.emit` selects on a `stopping` channel that `Stop` closes *before* it waits on `loopDone`. Without that, a consumer in the natural `for ev := range eng.Events() { … eng.Stop() }` shape — which stops draining the instant it calls `Stop` — parks `runLoop` in a blocking send on a full buffer, so it never re-enters its select to observe `ctx.Done` and `Stop` hangs forever. `runLoop` also clears `turnActive` on exit, so a connection dropped mid-turn cannot pin the engine into permanently rejecting `SendUserTurn` with `ErrTurnActive`.

**Turn interruption.** Codex retains the id returned by `turn/start` and sends `turn/interrupt` with both thread and turn ids; Claude writes Ctrl+C to its PTY and lets the Stop hook own the idle transition; API mode cancels only the current `Conversation.SendTurn` context. All three return `ErrNoActiveTurn` while idle and preserve their session-level context for the next message.

**Two approval protocol generations, different vocabularies** (validated against `codex app-server generate-json-schema`, codex-cli 0.144.1; see `console_engine_codex_approval.go`'s `approvalDecisionWire`):

| Generation | Methods | approve | approve_for_session | deny | abort |
|---|---|---|---|---|---|
| v2 | `item/commandExecution/requestApproval`, `item/fileChange/requestApproval` | `accept` | `acceptForSession` | `decline` | `cancel` |
| legacy | `execCommandApproval`, `applyPatchApproval` | `approved` | `approved_for_session` | `denied` | `abort` |

`decline`/`denied` means the command is not executed but the turn continues; `cancel`/`abort` also interrupts the turn. `serverRequest/resolved` is a *notification* (no id) meaning the server resolved a pending request elsewhere (or it timed out) — `codexEngine.runLoop` intercepts it ahead of `dispatchAppServerEvent` and drops the matching pending entry without replying. `ReplyApproval` drops a pending entry only *after* the reply is written: `ApprovalDecision` is a bare string, so an unmappable value (or a transport failure) must leave the id retryable rather than consuming it and leaving codex blocked on that JSON-RPC id forever.

**Why `item/permissions/requestApproval` is not decision-shaped.** Its response shape is `{permissions, scope, strictAutoReview}`, not `{decision: ...}` — it configures a permission grant, it doesn't approve/deny one action. `codexEngine.onServerRequest` does not special-case it; like every other unhandled server request (`item/tool/requestUserInput`, `mcpServer/elicitation/request`, `item/tool/call`, `account/chatgptAuthTokens/refresh`, `attestation/generate`, ...) it falls through to `client.replyError`, so codex is never left blocked on a request the engine does not implement. The autonomous backend's blanket `{"decision":"approved"}` auto-reply (`codex_appserver_backend.go`, only valid for the legacy pair and unreachable under `approvalPolicy=never`) is deliberately not reused here.

**Delegation blocking.** `appServerArgs()` keeps `--disable multi_agent --disable multi_agent_v2 --disable enable_fanout` for `codexEngine`: an app-server-spawned child is invisible to nrflo, so native delegation must stay disabled for server-owned console conversations.

### claudeEngine (console_engine_claude*.go)

`claudeEngine` drives a console conversation over the same PTY + Claude-hooks transport the autonomous `cli_interactive` backend uses — no headless `-p`/stream-json. `Start` writes per-session MCP and hook settings, creates a PTY keyed by `spec.SessionID`, and registers with `ConsoleHub`; PTY output is dropped because heartbeat, transcript text, approvals, and turn boundaries come from hooks. Its argv carries none of the managed-session bypass/deny-list/safety-hook flags.

**Hook wiring via ConsoleHub, not a direct channel.** Unlike `codexEngine` (JSON-RPC request/response over its own client), `claudeEngine` never talks to the CLI process directly for events — Claude's `--settings` hooks call `nrflo_server agent record-event --console`, which reaches the socket server, which calls into `spawner.ConsoleHub` (`console_hub.go`), which looks up the engine by session id and invokes one of four `consoleTarget` methods: `NotifySessionReady` (SessionStart → unblocks `SendUserTurn`'s TUI-ready wait), `NotifyTurnEnd` (Stop → flush transcript, clear `turnActive`, emit `EventTurnCompleted`, call `Sink.OnTurnComplete`), `NotifyContextLeft` (statusline via `agent.context_update` → `EventTokenUsage`), and `RequestApproval` (PreToolUse → the blocking approval path below). `ConsoleHub.Register`/`Unregister` run from `Start`/`Stop`; every hub method returns `handled=false` for a session with no registered engine, so autonomous sessions are unaffected.

**Timeout ladder (strictly increasing).** Server-side `RequestApproval` wait `consoleApprovalTimeout`=600s < the CLI's `agent record-event --console` select deadline `consoleHookDeadline`=630s (`cli/agent_hooks.go`) < its socket read deadline `consoleHookReadDeadline`=660s ≈ the settings PreToolUse hook's own `timeout`=660s (`BuildConsoleSettingsJSON`). The read deadline is the rung that is easy to miss: `Client.Execute`'s default is a hard 5 minutes — *below* the 600s approval wait — so the console path must call `ExecuteAndUnmarshalWithReadDeadline`. On the default, a human answering after 300s would have the socket read time out and deny the tool while the engine still recorded their "allow", silently diverging the decision from what claude did.

Every layer denies rather than erroring non-zero: a hook that exits non-zero or prints nothing leaves claude falling back to its own interactive permission prompt, which nobody can answer in a server-driven PTY. So a `--console` PreToolUse call always exits 0 and prints a deny `hookSpecificOutput` on *every* no-decision path — select deadline, transport error, and server-not-running/no-session (which return before the socket call at all, and so must deny explicitly instead of exiting silently the way an autonomous hook does).

**Turn delivery.** `SendUserTurn` mirrors `deliverPrompt`'s PTY discipline (`backend_interactive_helpers.go`): wait for SessionStart (`claudeSessionStartTimeout`=20s), persist the `user_input` row *before* writing so transcript-tailed assistant rows cannot land first, write the body, wait `claudeSubmitDelay`=150ms, then write the submit `\r`. Both waits matter — coalescing body+CR into one PTY read lets the TUI swallow the submit, leaving a turn typed but never sent (and `turnActive` stuck, since only the Stop hook clears it). `claudeBootstrapFloor`=1.5s applies only to the FIRST turn: it exists to let the TUI finish its initial paint, so charging it to every later turn is pure latency. All four durations are injectable engine fields, zeroed in tests (Rule 4).

**permissionDecision wire vocabulary.** `claudeDecisionWire` (`console_engine_claude_approval.go`) maps `ApprovalApprove`/`ApprovalApproveForSession`→`"allow"`, `ApprovalDeny`/`ApprovalAbort`→`"deny"` (abort carries reason `"aborted by user"`). Claude has no native PreToolUse equivalent of codex's `acceptForSession`, so `ApprovalApproveForSession` is remembered engine-side: `ReplyApproval` records the pending approval's tool name in a session allowlist and `RequestApproval` auto-allows that tool (by name — coarser than codex) without emitting an approval request. An unmappable decision still errors and leaves the pending id retryable, matching `codexEngine.ReplyApproval`'s drop-after-write rule. `decision` is deprecated for PreToolUse per the installed CLI's own docs; only `permissionDecision` is used there (Stop/PostToolUse/UserPromptSubmit keep the old `decision:block` shape untouched, see `cli/agent_hooks.go`'s `renderHookDecision`).

**Transcript tail offsets.** `flushTranscript` reconstructs the transcript path via the proc-free `claudeTranscriptPath(env, workDir, sessionID)` (`inband_rate_limit.go`, shared with `lastAssistantText`; it symlink-resolves workDir first — claude encodes its *resolved* cwd, so `/var/...` workdirs would otherwise never match on macOS) and tails it with the same byte-offset + only-complete-lines pattern as `socket/transcript_thinking.go`: a partial trailing line is left unconsumed, and `offset > size` restarts at 0 (rotated/truncated file). It runs on both a `tailLoop` ticker and hook-triggered flushes (`RequestApproval`, `NotifyTurnEnd`) so text surfaces promptly around tool calls and turn ends — which means concurrent flushes from three goroutines, so a dedicated `flushMu` serializes the whole read-process-advance sequence: two overlapping flushes would otherwise both start from the same offset and emit + persist every new line twice. Assistant `text` blocks emit `EventText` and persist via `emitAgentText` (category `"text"`); `thinking` blocks emit `EventThinking` only — no Sink row, so socket's `tailThinking` (triggered by the *same* Claude hooks on the *autonomous* path) stays the single writer of `"thinking"` rows and neither path double-inserts for a console session (`tailThinking` only ever fires from a `PreToolUse`/`PreCompact` hook call that reaches `handleAgentRecordEvent`, which console sessions also send, but the row category split means both can coexist without collision). `tool_use`/`tool_result` blocks are intentionally skipped — those already surface as `EventToolInvoke`/hook-driven events, not transcript rows.

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

Every child env starts from `HostEnvWithoutClaudeMarkers()` (`spawner_util.go`) — `os.Environ()` minus `CLAUDECODE`/`CLAUDE_CODE_*`. Inheriting a parent Claude Code session's markers makes a spawned claude treat itself as a nested child session (`CLAUDE_CODE_CHILD_SESSION` suppresses its project transcript JSONL entirely — verified on 2.1.211), starving the console transcript tailer and resume-based context save. Used by CLI/script/observer spawns, `api/handlers_pty.go` resume launches, and `console.chatEnv`; `tools_python` mirrors the rule inline (import cycle).

## Observer Agent

Entry point: `spawn_observer.go:23`. Called by `ObserverService.Launch` (service/observer.go) — bypasses orchestrator, layer, and phase; no `workflow_instances` row is created. The session row uses `agent_type=_observer`, `phase=observer`, `kind=observer`, and `observer_scope` set to the requested scope. Env vars injected in addition to the standard agent envelope: `NRF_OBSERVER=1`, `NRF_OBSERVER_SCOPE`, `NRF_PROJECT_ID`, and (when scope=workflow) `NRF_WORKFLOW_ID`. Terminates via the standard `CompleteInteractive` / `KillInteractive` interactive-wait path.
