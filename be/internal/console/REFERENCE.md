# console Package Reference

Uncapped overflow from [CLAUDE.md](CLAUDE.md). Read the relevant section before changing the flows it documents.

## Console-Chat Profiles

`Profile` (`profiles.go`) fields: `Name`, `DisplayName`, `Description`, `DefaultEngine`, `DefaultModelID`, `DefaultEffort`, `ContextBudgetTokens`, `RefineryDefault`, `SystemTemplateID`, `NativeToolPolicy`, `Catalogue []string`, `SiblingFlows bool`. `ProfileByName("")` returns the zero `Profile` (no restriction, no defaults, `SiblingFlows=false`) — the pre-profile behavior every existing chat-create call keeps by passing an empty profile name; `ProfileByName(unknown)` returns `ErrUnknownProfile`.

### t0-decider

`engine=claude`, `model=opus-5` (the 200k-context, non-`[1m]` row), `effort=xhigh`, `budget=50000`, `refinery=true`, `system_template_id=tier-t0-decider` (seeded by migration 000178), `native_tool_policy=none`. Catalogue (`t0DeciderCatalogue`): `delegate`, `get_delegation`, `workflow_run`, `workflow_get`, `workflow_list`, `workflow_continue`, `workflow_stop`, `dynamic_workflow`, `get_subworkflow`, `revise_plan`, `approve_plan`, `project_findings_add/_add_bulk/_append/_append_bulk/_get/_delete`, `ticket_create/_update/_add_dependency/_list/_get/_current`, `artifact_list`, `artifact_get`, `web_search`, `consult`. Deliberately absent: any fs/bash tool, `web_fetch`, `workflow_wait`/`workflow_retry_failed`/`project_list`/`project_status` (a T0 decider drives via `delegate`/`dynamic_workflow`, not by running or polling workflows directly).

### t0-hands

`engine=claude`, `model=sonnet-5` (already 1M-context natively, no `[1m]` row needed), `budget=150000`, `refinery=false`, `native_tool_policy=full`, `Catalogue=nil` (full console catalogue). Opened only via `OpenHandsSibling`, never created top-level by the picker's default flow (though the profile itself imposes no such restriction — the UI is what steers this).

### t0-bare

`engine=claude`, `model=opus-5`, `effort=xhigh`, `budget=30000`, `refinery=true`, `system_template_id=tier-t0-bare` (seeded by migration 000190), `native_tool_policy=none`. Catalogue (`t0BareCatalogue`, exactly 13 tools): `delegate`, `get_delegation`, `dynamic_workflow`, `get_subworkflow`, `revise_plan`, `approve_plan`, `workflow_run`, `workflow_list`, `workflow_get`, `workflow_continue`, `workflow_stop`, `ticket_list`, `ticket_current`. Narrower than `t0-decider`: no `project_findings_*`, `artifact_list`/`artifact_get`, `web_search`, `consult`, or `ticket_create`/`_update`/`_add_dependency`/`_get` — a pure-delegation profile that decides and delegates only, nothing else.

### Console-side delegation-guidance append

`buildChatEngineSpec` (`chat_spec.go`) computes the standard `vars` map once, renders `SystemTemplateID` into `spec.SystemPrompt` when set, then unconditionally calls `spawner.AppendDelegationGuidanceForTools(ctx, pool, spec.SystemPrompt, p.Catalogue, vars)` — the same guard (delegate-membership check + render `delegation-guidance` + `TrimSpace`-empty guard + `"\n\n"` join) the spawner's own api/api-via-cli/cli_interactive seams use, gated on `p.Catalogue` (the profile's enumerated tool-name list) rather than the resolved registry, so a nil/empty catalogue (no profile, `t0-hands`) is a no-op and only `t0-decider`/`t0-bare` (which enumerate `delegate`) see the append. `chat_service.go`'s `create()` and `chat_service_rotate.go`'s `rotate()` both pass `Catalogue: profile.Catalogue` into `chatSpecParams`.

### Threading into the engine

`chat_spec.go`'s `chatSpecParams` carries the profile's `NativeToolPolicy`/`ContextBudgetTokens`/`DefaultModelID`/`DefaultEffort` (populated by `chat_service.go`'s `create()` from `ProfileByName`); `buildChatEngineSpec` maps `NativeToolPolicy` onto the raw `spawner.EngineSpec` fields via `nativeToolFieldsForPolicy`: `"none"` → `NativeToolsCSV=model.NativeToolsNone` (claude `--tools ""`, `console_engine_claude.go`) + `Sandbox=model.SandboxReadOnly` (codex thread/start param, already threaded at `console_engine_codex.go:73`); `"full"`/`""` leave both empty (engine default). `ContextBudgetTokens` rides straight onto `EngineSpec.ContextBudgetTokens`. The api engine (`console_engine_api.go`) gates its native fs tools on `NativeToolPolicy` directly: `"none"` never adds them (overrides `api_native_tools_enabled`), `"full"` always adds them when a workdir is set, `""` keeps the existing global-gated behavior; its context-watcher budget is `spec.ContextBudgetTokens` when set, else the `context_budget_default` global (`watcherBudget`). `DefaultModelID`/`DefaultEffort` apply in `buildChatEngineSpec` only when the caller's own `ModelID`/`ReasoningEffort` are empty — an explicit create-time value always wins.

### Rotation

`ProactiveRestartConsoleThreshold(pool, maxContext, budget)` (`spawner/context_restart.go`) computes the usual percentage-of-window ceiling, then caps it at `budget` when `budget>0` — so t0-decider's 50k budget rotates a 200k-window claude chat at 50k tokens, well under the default 75% (150k) ceiling. `chat_service_rotate.go`'s `maybeRotate`/`rotate` resolve `ProfileByName(sess.Profile())` fresh each call and rebuild the registry (`BuildRegistry(d, profile.Catalogue)`) and spec (`NativeToolPolicy`/`ContextBudgetTokens`) identically to `create()`, so a rotated engine keeps the same restricted catalogue/budget as the one it replaced.

### Persistence

`agent_sessions.console_profile TEXT NOT NULL DEFAULT ''` (migration 000185) stores the profile name on the row; `model.AgentSession.ConsoleProfile` (plain `string`, not `sql.NullString` — the column is `NOT NULL`). `chat_catalog.go`'s `Catalog()` populates `types.ConsoleCatalog.Profiles` from `ListProfiles()` and stamps each live `ConsoleSessionOption.Profile` from the row; `GET /api/v1/console/chats` and `GET .../chats/{sid}` mirror this in their own response shapes (`handlers_console_chat_list.go`).

### HTTP/socket profile param

`POST /api/v1/console/chats` body and the `console.chat` socket method both accept an optional `profile` string, threaded to `ChatService.Create`/`CreateAuthenticated`. `handlers_console_tools.go`'s `catalogueForSession` resolves a `kind=console_chat` session's profile catalogue for the HTTP-mediated tool routes the claude/codex `agent mcp-external` bridge hits — the api engine's in-process registry (built directly in `chat_service.go`) is the only other place a profile catalogue is enforced, so both paths must stay filtered identically for a claude/codex t0-decider chat to actually be catalogue-restricted.

### Hidden-host consult

The console `consult` tool (`tools_consult.go`) routes through `Deps.Consultant` (`orchestrator.APIConsultant`, mirroring `APIDelegator`), which resolves `callerSessionID` to a project then calls `spawner.Spawner.ConsultHost(ctx, projectID, consultantID, question)` — the hidden-host counterpart to the session-bound `Consult`. Unlike `Consult` (which scopes the consultant lookup to the caller's own workflow instance's `WorkflowID`), `ConsultHost` resolves the consultant across the project's `agent_definitions` via `repo.AgentDefinitionRepo.FindConsultant` (project-local rows win over the reserved `__global__` namespace) — a console session has no single caller-known workflow to scope to. Both funnel into the shared `runConsult` (`spawner/consult_run.go`); a host call runs with no transcript (a console chat has no `AgentSession`-backed message history to summarize).

### Hidden-host dynamic_workflow / plan tools

`dynamic_workflow` (`tools_dynamic.go`) does **not** go through `apirun.SubworkflowRunner.StartDynamicWorkflow` (which requires a real, currently-active parent `workflow_instances` row via `o.runs[parentInstanceID]`) — a console session has neither. It instead calls `Deps.Orch.StartWorkflow(ctx, projectID, "", service.DynamicWorkflow, instructions, "project")`, the same top-level entrypoint `workflow_run` uses; `StartWorkflow` never sets `PlanAutoApprove`, so the run always suspends at the plan boundary for the caller to drive.

`get_subworkflow`/`revise_plan`/`approve_plan` (`tools_plan.go`) are project-guarded (`loadGuardedInstance`), not parent-ownership-guarded (`orchestrator.assertChildOwnership`) — a console session can poll/drive **any** project-owned instance, not just one it started. They call `service.NewPlanService(pool, clock, d.Orch)` directly (mirroring the `POST /api/v1/workflow-instances/{iid}/plan/*` REST handlers) instead of `env.Subworkflows`; `console.Orchestrator`'s `RunPlanner`/`ResumeAfterPlanApproval` methods exist so an `Orchestrator` value satisfies `service.PlannerRunner` directly (interface-to-interface conversion, no adapter type). `subworkflowStateFor` reproduces `orchestrator.GetSubworkflow`'s status/plan-payload mapping locally rather than reusing it, since that function is itself ownership-guarded.
