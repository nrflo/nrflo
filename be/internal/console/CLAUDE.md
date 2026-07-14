# console Package

Server-owned tool catalogue + dispatcher for `kind='console'` `agent_sessions` (see [api/CLAUDE.md](../api/CLAUDE.md#console-sessions)), served over `GET`/`POST /api/v1/console/tools*`.

## Profile Invariant

The console `apirun.ToolEnv` has no `WorkflowInstanceID` — a console session isn't bound to a run — so the profile (`registry.go`) includes **only session-independent tools**: an explicit allowlist of `tools_builtin.Builtins()` entries (`reusedBuiltins()`) plus console-only handlers (`tools_workflow*.go`, `tools_project.go`, `tools_artifact.go`, `tools_deep_research.go`) that take an explicit `instance_id`/`ticket_id` instead of reading it off the env. Every session-bound lifecycle/findings/artifact-write tool (`agent_*`, `findings_*`, `emit_findings`, `workflow_skip`, `chain_next_*`, sub-workflow/plan tools, `consult`, `read_document`, `artifact_add`) is excluded — `BuildRegistry` errors if an allowlisted name goes missing from `Builtins()` (rename guard), never silently drops it.

Because those ids are caller-supplied, **every** tool taking an `instance_id` is project-guarded: console handlers via `loadGuardedInstance` (`helpers.go`), and the reused `workflow_continue`/`workflow_fail` builtins via the orchestrator's `APIWorkflowControl` adapter (see [orchestrator/CLAUDE.md](../orchestrator/CLAUDE.md#misc)). A console token must never act on another project's instance.

## ArtifactSvc Nil in ToolEnv

`env.go`'s `NewToolEnv` leaves `ToolEnv.ArtifactSvc` nil: `artifacts.workflow_instance_id` is `NOT NULL REFERENCES workflow_instances(id)` with `foreign_keys(1)` on, and a console session owns no instance, so any write through the shared `web_fetch`/etc. artifact path would FK-fail after already writing the blob. `web_fetch` takes its documented nil-artifact-store branch instead. The console `artifact_list`/`artifact_get` tools (`tools_artifact.go`) hold their **own** `*service.ArtifactService` from `Deps` and take an explicit `instance_id`, so they are unaffected.

## Dispatch

`Dispatch(ctx, reg, env, name, args)` (`dispatch.go`) is the one call site `api.handleCallConsoleTool` uses; `ErrToolNotFound` maps to the endpoint's 404. `Specs(reg)` backs the catalogue endpoint, sorted by name.

## Console drivers

`ConsoleDriver` (`driver.go`: `Name`/`Probe`/`Prepare(LaunchInput) (LaunchSpec, func(), error)`) is what `nrflo_server console` (`cli/console.go`) uses to launch a native claude/codex CLI locally as a **human** session — `GetDriver` is the only provider-name switch. The launched CLI reaches nrflo through `agent mcp-external` over a console session the `console` command mints and injects (`NRFLO_CONSOLE_TOKEN`/`NRFLO_CONSOLE_SESSION_ID`). The cc96eed6 managed-session boundary (`--dangerously-skip-permissions`, `--disallowedTools`, a safety-hook `--settings`, `--dangerously-bypass-approvals-and-sandbox`) applies only to spawner-managed sessions and is deliberately absent from every driver here.

`--model` resolves against the `cli_models` registry, which supplies `reasoning_effort`/`fallback_models` alongside `mapped_model`: [REFERENCE.md](REFERENCE.md#console-driver-model-resolution).

## Console chat sessions

`ChatService` (`chat_service.go` + `chat_session.go`/`chat_spec.go`/`chat_sink.go`/`chat_events.go`) owns `kind='console_chat'` `agent_sessions` lifecycle: it mints the row + bearer token, builds a `spawner.EngineSpec`, starts a `spawner.ConsoleEngine` (via an injectable factory defaulting to `spawner.GetConsoleEngine`), tracks the turn state machine (a second message while a turn is in flight is rejected without an engine round-trip), and pumps `EngineEvent`s onto the session's WS channel (`hub.BroadcastSession`, see [ws/CLAUDE.md](../ws/CLAUDE.md)). Tools reach the server the same way a human console session's do: the engine's `--mcp-config`/profile already points at `agent mcp-external`, which adopts this pre-minted session from `NRFLO_CONSOLE_TOKEN`/`NRFLO_CONSOLE_SESSION_ID` and proxies to the same `console.BuildRegistry`+`NewToolEnv` tool routes (`requireConsoleSession` accepts both `console` and `console_chat` kinds). Engines hold no `processInfo`, so stall/nudge/restart are structurally unreachable — this is not a kind check anywhere in the spawner/orchestrator.

An engine that dies is not a stuck chat: `pumpChatEvents` ends the turn on every `EventError` (each one is turn-terminal), and when `Events()` closes — Stop, or the engine dying on its own — it tears the session down (drop from the map, `CloseConsoleChat`, killing the bearer token) and pushes `console_chat.turn` state=idle **last**, so a subscriber that sees it knows the row is already closed. Without that, an engine that died mid-turn would pin the turn `running` and 409 every later message forever.

The engine name a chat row was started with is persisted in `agent_sessions.console_engine`, so `GET /api/v1/console/chats` does not depend on the in-memory session map. `pumpChatEvents` is the single writer of the `console_chat.approval_resolved`/`console_chat.thinking` session pushes for both engines — an engine's `RequestApproval`/`ReplyApproval`/resolved-notification paths only emit `spawner.EventApprovalResolved`, exactly once per registered id (whoever *drops* the pending entry owns the emit, so a reply racing a timeout cannot double-resolve) and never push WS/audit themselves. The pump normalizes the spawner's `approve`/`approve_for_session`/`deny`/`abort` vocabulary down to the `allow`/`deny` that the REST reply route accepts, so a client speaks one vocabulary in both directions; token usage is pushed there too, but a claude chat's also fans out from `socket/handler_context.go`, so that one event legitimately arrives twice (idempotent).
