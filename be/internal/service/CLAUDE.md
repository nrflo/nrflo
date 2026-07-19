# Service Package

Business logic layer separating domain logic from HTTP/socket handlers.

## File-to-Responsibility Mapping

| File | Domain |
|------|--------|
| `project.go` | Project CRUD operations |
| `ticket.go` | Ticket CRUD, close/reopen, search, `ValidateRunnable` (rejects blocked tickets) |
| `workflow.go` | Workflow operations (ticket + project scope): init, start/complete phase, state queries |
| `workflow_defs.go` (+ `_read`/`_validate`/`_agents`) | Workflow definitions CRUD (phases derived from agent_definitions at read time); validates `next_workflow_on_success` (no self-ref, same project, project-scoped), `callable_as_subworkflow` (project scope + no purge — sub-runs are ticketless) and `finalize_success`/`finalize_failure`/`pause_event` slots (`command`/`script_id` mutually exclusive; `script_id` must resolve to a `kind='agent'` python_scripts row) |
| `workflow_config.go` | `BuildSpawnerConfig`: phases from agent_definitions (layer ASC, id ASC) |
| `workflow_types.go` | Type definitions: `WorkflowDef`, `PhaseDef`, `RestartDetail` |
| `layer_policy.go` | `ParseLayerPolicy`, `LayerPolicy.Required(denom)`, `ValidateLayerPolicy`; kinds: `any`, `all`, `quorum:N`, `percent:P` |
| `workflow_layer_policy.go` | `WorkflowLayerPolicyService`: Get/Set/Delete; validates quorum ≤ agent count |
| `workflow_validation.go` | `validateLayerConfig` (layer >= 0), `ValidateScopeType`, `ValidateGroups` |
| `workflow_response.go` | V4 response building: active agents, history, findings aggregation, phase status |
| `workflow_restart_details.go` | Restart detail loading: duration, context, message count from continued sessions |
| `agent.go` | Agent session operations; `Fail`/`Continue` return `(sessionID, error)` |
| `agent_definition.go` (+ `_read.go`) | Agent definition CRUD; validates `layer >= 0`; `execution_mode` cli_interactive/api/script (default cli_interactive); script requires `python_script_id` in same project (rejects `kind='tool'` binding); validates `validation_commands` (max 20, ≤1024 bytes, no empty); `consultant=true` requires `execution_mode='api'`. Consultant defs (and `node_role != 'static'` — `planner`/`fanout_template`) never become phases: `repo.AgentDefinitionRepo.ListExecutable` excludes them from the execution graph, v4 read model, and layer-gap/pass-policy/quorum counts. `reasoning_effort` (nullable, NULL=inherit model row) is validated against the model row on create/PATCH; `native_tools` (claude-only CSV, `none` must be sole entry) and `sandbox` (codex-only enum) are hard-validated against the model row's provider + execution mode on create/PATCH with merged effective values; `system_template_id` (`''`=global override rules, else must resolve to a `type='injectable'` `default_templates` row) selects the per-role system prompt |
| `trace.go` (+ `_lanes`/`_markers`/`agent_tool_span`) | Read-time trace for `GET /workflow-instances/{iid}/trace`: lanes merged by chain root, layer bands, markers from agent_messages/findings_history (tool spans close via payload `ended_at`) + `lifecycle` markers and nudge/stop-block counters from session columns, children via `parent_instance_id` |
| `findings.go` | Findings add/append/get/delete |
| `findings_emit.go` + `finding_schema.go` | `FindingsService.Emit`: validates a finding `value` against the workflow's per-key schema (session → workflow → `workflows.finding_schemas`), stores on success, else errors with the configured example. `ValidateFindingSchemas`/`compileJSONSchema` (Draft 2020) validate schema definitions at workflow-def create/update |
| `chain.go` | Chain build, dependency expansion, topological sort, cycle detection |
| `chain_preview.go` | `PreviewChain`, custom order validation |
| `chain_append.go` | Append tickets to running chains |
| `chain_remove.go` | Remove pending items from running chains |
| `daily_stats.go` | Daily stats computation (workflow_agent sessions only); `cost_estimate` sums `agent_sessions.cost_estimate` alongside `tokens_spent` |
| `console.go` | Console session lifecycle: Create (kind='console' row + bearer), Close, SweepIdle (`console_idle_ttl_hours`) |
| `git.go` | Paginated commit listing and commit detail (os/exec) |
| `worktree.go` (+ `_context.go`) | Git worktree lifecycle: Setup, MergeAndCleanup, Cleanup; Setup seeds untracked/gitignored agent context into the worktree (CLAUDE.md/AGENTS.md copied, `.claude` dirs symlinked, any depth, absent-only) |
| `system_agent_definition.go` (+ `_read.go`) | System agent definition CRUD (global) |
| `default_template.go` | Default template CRUD (global, readonly enforcement) |
| `model.go` + `model_update.go` + `model_reasoning.go` (+ `model_inuse.go`) | Unified model CRUD: one provider row enables CLI/API through non-empty mode model IDs and carries per-mode contexts/effort lists, one default effort, and nullable per-MTok `price_*` columns. `IsValidModelForMode` validates enabled mode support for definitions; readonly rows only accept `default_effort`/`fallback_models`, fallback models are anthropic-only and capped at 3. `ModelInUseCheck` blocks disable/delete while any agent/system def, or observer setting (global/project `observer_model` config, `workflows.observer_model`) references the id; clearing a mode's model id is blocked per execution mode (observers count as cli) |
| `plan_validate_premium.go` | `PlanModelTierClass` (single classifier, Rule 6): consults `model.PriceClass()` (price_in thresholds) when the row has seeded pricing, else falls back to name-class (opus/fable=premium, haiku=cheap, else mid) |
| `global_settings.go` | Key-value settings (wraps `pool.GetConfig`/`SetConfig`/`GetProjectConfig`/`SetProjectConfig`) |
| `error_service.go` | `RecordError` (UUID, clock, DB insert, WS broadcast), `ListErrors` (paginated) |
| `notification.go` | Notification channel CRUD + secret masking + TestSend + ListDeliveries |
| `snapshot.go` | WS snapshot provider |
| `insights.go` | `Summary`/`EditRate`/`Throughput`; `ParseRange` (7d/30d), `ParseBucket` (1h/6h/1d) |
| `workflow_chain.go` | `WorkflowChainService`: chain+step CRUD; validates dense positions, step 0 project-scope, workflow_name resolves |
| `workflow_chain_run.go` | `WorkflowChainRunService`: CreateRun, GetRunDetail, ListRuns, SetNextStepInstructions, SetNextStepTicket |
| `python_script.go` | `PythonScriptService`: CRUD + ListByKind/ListTools; rows discriminated by `kind` (`agent` default, or `tool`); validates `file_path` (absolute, `.py`); `kind=tool` requires `tool_description` + compiled `input_schema` (Draft2020), `timeout_sec ∈ [1,600]`; `kind` immutable |
| `user_service.go` | `UserService.Delete`: self-delete → system-user → last-admin checks; system users flagged via `users.system=1` |
| `python_script_validate.go` | `Validate(ctx, code)`: runs `python3` AST parse with 5s timeout; degrades gracefully if python3 absent |
| `project_env_var.go` | `ProjectEnvVarService`: List/Upsert/Delete; validates name regex, reserved names, 4096-byte value cap |
| `artifact.go` | `ArtifactService`: StageUpload/ReadUploadMeta/CancelUpload, AttachInputArtifacts (staged→persistent, transactional rollback), List/Get/Open/Delete (broadcasts `artifact.deleted`) |
| `purge.go` | `PurgeService.PurgeInstanceTraceIfEnabled`: when `purge_on_completion` is set, redacts `agent_sessions` sensitive columns, deletes messages/findings/findings_history/artifacts/errors, clears caller columns. One tx; audits + broadcasts; invoked from orchestrator terminal hooks |
| `subworkflow.go` | Sub-workflow guard config keys/defaults (tools_enabled on; depth 3, children 6, invocations 25; project>global) |
| `plan_auto.go` | `dynamic_workflow_auto_enabled` gate (project>global, default off) + `DynamicWorkflow` name const |
| `dynamic_seed.go` (+ `_data`/`_prompts_*`/`_planner`/`_schemas.go`) | `EnsureGlobalDynamicWorkflow` — see "Global workflows" below |
| `plan.go` (+ `plan_*`/`workflow_instance_nodes`.go) | Plan lifecycle: `PlanService` draft/revise/approve→materialize/cancel/TTL-sweep; schema+semantic validation; template-library availability recheck; hash-stamped idempotent `Materialize`; server-owned reserved key `_workflow_plan`. Full mechanics: [REFERENCE.md](REFERENCE.md#plan-lifecycle-plango--siblings) |
| `workflow_export.go` + `workflow_export_import.go` | `WorkflowExportService`: `Export` builds a v1.0 `types.WorkflowBundle` (strips project_id/workflow_id, dedupes scripts); `CheckImport` probes ID conflicts; `Import` applies `overwrite`/`rename`/`cancel` |

## Per-project env vars

`project_env_vars` table; CRUD under `/api/v1/projects/{id}/env-vars` (writes admin-only); validated names/reserved set/4KB cap; injected into every backend's spawn env after nrflo-controlled vars. Details: [REFERENCE.md](REFERENCE.md#per-project-env-vars).

## Global workflows

A workflow definition is global when `workflows.is_global=1`. Global defs are project-agnostic: `ListWorkflowDefs` (`workflow_defs_read_list.go`) unions them into **every** project's listing with **local-name precedence** (a project-local def shadows a same-named global one), so they are selectable/runnable from any project while execution stays project-scoped. `GetWorkflowDef` (`workflow_defs_read.go`), `loadFindingSchemas` (`finding_schema.go`), and the orchestrator's `resolveWorkflowDef` resolve a name under the selected project, then fall back to the global namespace.

Global defs are stored under the reserved `GlobalProjectID` (`"__global__"`, `workflow_reserved.go`) — a hidden project (kept out of `ProjectRepo.List`/`ProjectService.List`) that keeps the composite `(project_id, id)` keys + child FKs intact. It is also a **runnable** project (a scratch `root_path`) so it's the execution home for project-agnostic tool runs with no real project in scope (e.g. via the `mcp-external` proxy). UI surfaces global defs in run pickers but hides them from workflow management (run-only, admin-managed); `WorkflowExportService.Export` excludes them. `workflow_instances.def_project_id` stamps the project the definition resolved under (`__global__` for globals) and carries the workflows FK, while `project_id` (the executing project) FKs to `projects` — so a global def runs under any real project (migration `000165`).

The bundled global workflow (`dynamic`) is seeded idempotently at startup by `EnsureGlobalDynamicWorkflow`; `__global__` mutation is admin-only. Seed contents + guards: [REFERENCE.md](REFERENCE.md#global-workflow-seeds).

## Constructor Pattern

Most service constructors take `(pool *db.Pool, clk clock.Clock)`. Pass `clock.Real()` in prod; `clock.NewTest(fixedTime)` in tests.

**Exception:** `NewAgentDefinitionService(pool, clk, modelSvc, pythonScriptRepo)` and `NewSystemAgentDefinitionService(pool, clk, modelSvc)` require the shared `*ModelService`; definition validation selects `cli_efforts` or `api_efforts` from the row according to execution mode.

Workflow/agent definition CRUD routes: see [api/CLAUDE.md](../api/CLAUDE.md).
