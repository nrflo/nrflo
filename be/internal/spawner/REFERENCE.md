# Spawner Reference

Deep mechanics for this package. The auto-loaded map lives in [CLAUDE.md](CLAUDE.md).

### Codex app-server backend

`codexAppServerBackend` (`codex_appserver_backend.go`) drives `codex app-server` over newline-delimited JSON-RPC stdio (`codex_appserver_client.go`), spawned with `--disable` flags blocking native delegation (`appServerArgs()`) — codex 0.133 emits no PTY hooks (openai/codex#21639) and no rollout JSONL, so app-server is the only structured channel. Events map to the standard `Sink` via `dispatchAppServerEvent` (`codex_appserver_events.go`): agentMessage→text, command/webSearch→tool, mcpToolCall→invoke+result, `thread/tokenUsage`→`context_left`, turn lifecycle→heartbeat, typed rate-limit. Completion stays socket/DB-driven; idle/nudge re-issues a `turn/start` with the `finish-reminder`. `SupportsResume()`/`SupportsTakeControl()` are both false. `CodexAdapter` is still used for model mapping + `ClassifyExit`; its PTY methods are unused.

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
