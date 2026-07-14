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

`--model` resolves against the `cli_models` registry (`resolveCLIModel`, `cli/console_client.go`): a matching enabled row supplies `mapped_model` **plus `reasoning_effort`/`fallback_models`** — registry ids are many-to-one on `mapped_model` (`codex_gpt55_high`/`_normal` both map to `gpt-5.5`), so dropping effort would silently launch a weaker model than the user named. A row belonging to the other `cli_type`, or a disabled one, errors before launch; an id absent from the registry falls back to the driver's own `adapter.MapModel`/`GetReasoningEffort`. Claude takes effort as `--effort`/`--fallback-model` (as the managed path does); the codex TUI has no effort flag, so it takes `-c model_reasoning_effort="<v>"` (it cannot be appended to the profile `config.toml`, which ends in a `[projects."<dir>"]` table).
