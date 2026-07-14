# console Package

Server-owned tool catalogue + dispatcher for `kind='console'` `agent_sessions` (see [api/CLAUDE.md](../api/CLAUDE.md#console-sessions)), served over `GET`/`POST /api/v1/console/tools*`.

## Profile Invariant

The console `apirun.ToolEnv` has no `WorkflowInstanceID` — a console session isn't bound to a run — so the profile (`registry.go`) includes **only session-independent tools**: an explicit allowlist of `tools_builtin.Builtins()` entries (`reusedBuiltins()`) plus console-only handlers (`tools_workflow*.go`, `tools_project.go`, `tools_artifact.go`, `tools_deep_research.go`) that take an explicit `instance_id`/`ticket_id` instead of reading it off the env. Every session-bound lifecycle/findings/artifact-write tool (`agent_*`, `findings_*`, `emit_findings`, `workflow_skip`, `chain_next_*`, sub-workflow/plan tools, `consult`, `read_document`, `artifact_add`) is excluded — `BuildRegistry` errors if an allowlisted name goes missing from `Builtins()` (rename guard), never silently drops it.

Because those ids are caller-supplied, **every** tool taking an `instance_id` is project-guarded: console handlers via `loadGuardedInstance` (`helpers.go`), and the reused `workflow_continue`/`workflow_fail` builtins via the orchestrator's `APIWorkflowControl` adapter (see [orchestrator/CLAUDE.md](../orchestrator/CLAUDE.md#misc)). A console token must never act on another project's instance.

## ArtifactSvc Nil in ToolEnv

`env.go`'s `NewToolEnv` leaves `ToolEnv.ArtifactSvc` nil: `artifacts.workflow_instance_id` is `NOT NULL REFERENCES workflow_instances(id)` with `foreign_keys(1)` on, and a console session owns no instance, so any write through the shared `web_fetch`/etc. artifact path would FK-fail after already writing the blob. `web_fetch` takes its documented nil-artifact-store branch instead. The console `artifact_list`/`artifact_get` tools (`tools_artifact.go`) hold their **own** `*service.ArtifactService` from `Deps` and take an explicit `instance_id`, so they are unaffected.

## Dispatch

`Dispatch(ctx, reg, env, name, args)` (`dispatch.go`) is the one call site `api.handleCallConsoleTool` uses; `ErrToolNotFound` maps to the endpoint's 404. `Specs(reg)` backs the catalogue endpoint, sorted by name.
