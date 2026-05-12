# apirun Package

> **Note:** Only reachable when the server starts with `--mode=api`. In default `--mode=cli`, `prepareSpawn` returns `api_mode_disabled` before constructing a Runner.

In-process tool-use loop for API-mode agents. Files: `runner.go` (turn loop), `interfaces.go` (MessageSink/ProcState/AgentSvc/ErrorRecorder surfaces), `tool.go` (ToolHandler/TerminalSignal/Registry), `registry.go` (ResolveRegistry), `secret_resolver.go` (secret deref), `sink.go` (event coalescing), `errors.go` (error classification), `provider/` (Anthropic streaming impl + mock), `tools_builtin/` (builtin handlers), `tools_http/` (HTTP tool handler).

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

Each terminal handler also calls the corresponding `AgentService` method, so DB row + WS broadcast happen identically to CLI agents.

## Builtins

Builtin tool handlers registered in `tools_builtin/builtins.go`; run `grep -n Register tools_builtin/*.go` for the canonical list.

## HTTP Tool Handler

`tools_http.New(client)` returns a factory bound to a shared `http.Client`. Handlers POST `{"tool":<name>,"input":<input>,"context":{...}}` to `def.Endpoint` with timeout (`def.TimeoutSec`, default 30s), auth per `def.AuthMethod` (none/bearer_env/bearer_secret_ref), 5xx retry once, 16 KB body cap.

## Per-Agent Registry Resolution

`ResolveRegistry(toolsCSV, builtins, httpDefs, httpFactory, manifestProvider)` composes builtins → manifest tools → HTTP defs. Glob matching: `""` = empty registry; `"*"` = all; `"findings.*"` = prefix glob. No match → spawn fails with `no tools matched`. Name collision → spawn fails with `collides with` error. HTTP scope: `project_id IS NULL OR == agent.project_id` AND `workflow_id IS NULL OR == agent.workflow_name`.

Manifest tools (`tools_manifest/`): built when `APIMode && CustomerConfigDir != "" && PythonRunner != nil`. Per-tool: validate input → `python.Runtime.Invoke` → insert `ToolDispatch` row → broadcast `tool.dispatched` → optional `ReviewItem` insert when `tool.Review && status==success`. Manifest cached by `tool_manifest.yaml` mtime (`loadManifestCached` in spawner).

## Wiring

`prepareSpawn` (api branch) calls `loadAPIHTTPToolDefs` + (optionally) `loadManifestCached` + `apirun.ResolveRegistry` → `prep.apiTools/apiHandlers`. `apiBackend.Start` builds an `apirun.Runner` in a goroutine. `mapFinalStatus` maps exit status: PASS→(pass,implicit), FAIL→(fail,api_error), CONTINUE→(continue,api_continue), CALLBACK→(callback,callback), CANCELLED→(fail,cancelled). See `spawner/api_backend.go`.

## Low-Context Behavior

`context_save.go` forces `useAgentSave=true` for API agents (resume path is Claude-CLI-only). `apiBackend.Kill` cancels runner ctx → saver agent summarizes history → `relaunchForContinuation` with `${PREVIOUS_DATA}`.

## Stall Detection

`runner.go`/`sink.go` call `TrackMessage` on each text/tool-use event, identical to CLI agents. Stall detection in `stall_restart.go`; cap 15 restarts.

Run `make test-pkg PKG=spawner/apirun`.
