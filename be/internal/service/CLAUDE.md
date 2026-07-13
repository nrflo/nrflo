# Service Package

Business logic layer separating domain logic from HTTP/socket handlers.

## File-to-Responsibility Mapping

| File | Domain |
|------|--------|
| `project.go` | Project CRUD operations |
| `ticket.go` | Ticket CRUD, close/reopen, search, `ValidateRunnable` (rejects blocked tickets) |
| `workflow.go` | Workflow operations (ticket + project scope): init, start/complete phase, state queries |
| `workflow_defs.go` (+ `_read`/`_validate`/`_agents`) | Workflow definitions CRUD (phases derived from agent_definitions at read time); validates `next_workflow_on_success` (no self-ref, must exist in same project, must be project-scoped), `callable_as_subworkflow` (requires project scope + no purge — sub-runs are ticketless and purge deletes the result before readback) and `finalize_success`/`finalize_failure`/`pause_event` slots (same rules: `command` vs `script_id` mutually exclusive; any `script_id` must resolve to a `python_scripts` row of `kind='agent'`) |
| `workflow_config.go` | `BuildSpawnerConfig`: phases from agent_definitions (layer ASC, id ASC) |
| `workflow_types.go` | Type definitions: `WorkflowDef`, `PhaseDef`, `RestartDetail` |
| `layer_policy.go` | `ParseLayerPolicy`, `LayerPolicy.Required(denom)`, `ValidateLayerPolicy`; kinds: `any`, `all`, `quorum:N`, `percent:P` |
| `workflow_layer_policy.go` | `WorkflowLayerPolicyService`: Get/Set/Delete; validates quorum ≤ agent count |
| `workflow_validation.go` | `validateLayerConfig` (layer >= 0), `ValidateScopeType`, `ValidateGroups` |
| `workflow_response.go` | V4 response building: active agents, history, findings aggregation, phase status |
| `workflow_restart_details.go` | Restart detail loading: duration, context, message count from continued sessions |
| `agent.go` | Agent session operations; `Fail`/`Continue` return `(sessionID, error)` |
| `agent_definition.go` | Agent definition CRUD; validates `layer >= 0`; `execution_mode` cli_interactive/api/script (default cli_interactive); script requires `python_script_id` in same project (rejects `kind='tool'` binding); validates `validation_commands` (max 20, ≤1024 bytes, no empty); `consultant=true` requires `execution_mode='api'`. Consultant defs (and `node_role != 'static'` — `planner`/`fanout_template`) never become phases: `repo.AgentDefinitionRepo.ListExecutable` excludes them from the execution graph, v4 read model, and layer-gap/pass-policy/quorum counts |
| `trace.go` (+ `_lanes`/`_markers`/`agent_tool_span`) | Read-time trace for `GET /workflow-instances/{iid}/trace`: lanes merged by chain root, layer bands, markers from agent_messages/findings_history (tool spans close via payload `ended_at`) + `lifecycle` markers and nudge/stop-block lane counters derived from session columns, children via `parent_instance_id` |
| `findings.go` | Findings add/append/get/delete |
| `findings_emit.go` + `finding_schema.go` | `FindingsService.Emit`: validate a finding `value` against the workflow's per-key schema (resolved via session → workflow → `workflows.finding_schemas`), store on success, else error with the configured example. `ValidateFindingSchemas`/`compileJSONSchema` (Draft 2020, shared with python tool `input_schema`) validate schema definitions at workflow-def create/update |
| `chain.go` | Chain build, dependency expansion, topological sort, cycle detection |
| `chain_preview.go` | `PreviewChain`, custom order validation |
| `chain_append.go` | Append tickets to running chains |
| `chain_remove.go` | Remove pending items from running chains |
| `daily_stats.go` | Daily stats computation |
| `git.go` | Paginated commit listing and commit detail (os/exec) |
| `worktree.go` | Git worktree lifecycle: Setup, MergeAndCleanup, Cleanup |
| `system_agent_definition.go` | System agent definition CRUD (global) |
| `default_template.go` | Default template CRUD (global, readonly enforcement) |
| `cli_model.go` + `model_reasoning.go` | CLI model CRUD; `model_reasoning.go` holds both effort validators: `xhigh` only for claude-opus-4-7/4-8 or claude-sonnet-5, `ultra` only for codex gpt-5.6-sol/terra (rejected for API models); `normalizeFallbackModels` caps the claude-only `fallback_models` chain at 3; readonly rows only accept `reasoning_effort` + `fallback_models` updates |
| `api_model.go` | API model CRUD (provider: anthropic/openai); effort rules in `model_reasoning.go:validateAPIReasoningEffort`; readonly rows only accept `reasoning_effort` updates; `IsValidModel` used by `agent_definition.go` and `system_agent_definition.go` for api-mode validation |
| `global_settings.go` | Key-value settings (wraps `pool.GetConfig`/`SetConfig`/`GetProjectConfig`/`SetProjectConfig`) |
| `error_service.go` | `RecordError` (UUID, clock, DB insert, WS broadcast), `ListErrors` (paginated) |
| `notification.go` | Notification channel CRUD + secret masking + TestSend + ListDeliveries |
| `snapshot.go` | WS snapshot provider |
| `insights.go` | `Summary`/`EditRate`/`Throughput`; `ParseRange` (7d/30d), `ParseBucket` (1h/6h/1d) |
| `workflow_chain.go` | `WorkflowChainService`: chain+step CRUD; validates dense positions, step 0 project-scope, workflow_name resolves |
| `workflow_chain_run.go` | `WorkflowChainRunService`: CreateRun, GetRunDetail, ListRuns, SetNextStepInstructions, SetNextStepTicket |
| `python_script.go` | `PythonScriptService`: Create/Get/List/ListByKind/ListTools/Update/Delete; rows are discriminated by `kind` (`agent` default, or `tool`); validates `file_path` (absolute, exists, `.py`); for `kind=tool` requires `tool_description`, compiles `input_schema` via santhosh-tekuri jsonschema Draft2020, enforces `timeout_sec ∈ [1,600]` (default 30); `kind` is immutable on update |
| `user_service.go` | `UserService.Delete`: self-delete → system-user → last-admin checks; system users flagged via `users.system=1` |
| `python_script_validate.go` | `Validate(ctx, code)`: runs `python3` AST parse with 5s timeout; degrades gracefully if python3 absent |
| `project_env_var.go` | `ProjectEnvVarService`: List/Upsert/Delete; validates name regex, reserved names, 4096-byte value cap |
| `artifact.go` | `ArtifactService`: StageUpload (tmp staging), ReadUploadMeta, CancelUpload, AttachInputArtifacts (moves staged uploads to persistent storage, transactional rollback), List, Get, Open, Delete (broadcasts `artifact.deleted`) |
| `purge.go` | `PurgeService.PurgeInstanceTraceIfEnabled`: when `purge_on_completion` is set, redacts `agent_sessions` sensitive columns and deletes messages/findings/findings_history/artifacts/errors, clears caller columns. One tx; audits `workflow.purged` + broadcasts `EventWorkflowPurged`. Invoked from orchestrator terminal hooks. |
| `subworkflow.go` | Sub-workflow guard config keys/defaults (tools_enabled on; depth 3, children 6, invocations 25; project>global) |
| `plan_auto.go` | `dynamic_workflow_auto_enabled` gate (project>global, default off) + `DynamicWorkflow` name const |
| `dynamic_seed.go` (+ `_data`/`_prompts_*`/`_planner`/`_schemas.go`) | `EnsureGlobalDynamicWorkflow`: seeds the `dynamic` workflow's fanout_template catalog + workflow-local planner def |
| `plan.go` (+ `plan_revise`/`plan_manifest`/`plan_validate[_refs]`/`plan_schema`/`plan_templates`/`plan_limits`/`plan_materialize`/`plan_status`/`workflow_instance_nodes`.go) | Plan lifecycle: `PlanService` (draft/revise/approve→materialize/cancel/TTL-sweep), manifest v1 types+hash, `ValidatePlanManifest` (one aggregated error), template-library resolution + enabled-model recheck, `plan_max_*`/`plan_draft_ttl_min` caps. `Materialize` writes an approved revision's nodes into `workflow_instance_nodes`/`_layer_policies` exactly once (hash-stamped conditional UPDATE); `DerivePlanInstanceStatus`/`IsPlanDriven`/`EffectivePhases` are the orchestrator's plan-boundary primitives — see [orchestrator/CLAUDE.md](../orchestrator/CLAUDE.md). Every `Revise` re-syncs an already plan-suspended instance's status (guarded: never clobbers a run executing static layers), so a poller sees `waiting_input`→`waiting_approval` once questions are answered. Reserved key `_workflow_plan` is server-owned, resolved ahead of `finding_schemas` — see `findings_emit.go` |
| `workflow_export.go` + `workflow_export_import.go` | `WorkflowExportService`: `Export` builds a v1.0 `types.WorkflowBundle` (strips project_id/workflow_id, dedupes python scripts); `CheckImport` probes ID conflicts; `Import` applies `overwrite`/`rename`/`cancel`. Python scripts get fresh IDs; agent `PythonScriptID` remapped before `CreateAgentDef`. |

## Per-project env vars

Stored in `project_env_vars` table (migration 000095). CRUD under `GET|PUT|DELETE /api/v1/projects/{id}/env-vars[/{name}]` (`handlers_project_env_vars.go`; writes admin-only). `ProjectEnvVarService` (`project_env_var.go`) validates: name matches `^[A-Za-z_][A-Za-z0-9_]*$`, not in reserved set (`NRFLO_PROJECT`, `NRFLO_AGENT_TOKEN`, `NRF_SESSION_ID`, `NRF_WORKFLOW_INSTANCE_ID`, `PATH`, `HOME`, etc.), value ≤ 4096 bytes. At workflow start, `orchestrator.loadProjectEnv` loads vars into `spawner.Config.ProjectEnv`; `prepareSpawn`/`prepareScriptSpawn` append them after nrflo-controlled vars for all backends (cli_interactive/api/script).

## Global workflows

A workflow definition is global when `workflows.is_global=1` (migration `000146`). Global defs are project-agnostic: `ListWorkflowDefs` (`workflow_defs_read_list.go`) unions them into **every** project's listing with **local-name precedence** (a project-local def shadows a same-named global one), so they are selectable/runnable from any project while execution stays project-scoped. `GetWorkflowDef` (`workflow_defs_read.go`), `loadFindingSchemas` (`finding_schema.go`), and the orchestrator's `resolveWorkflowDef` resolve a name under the selected project, then fall back to the global namespace.

Global defs are stored under the reserved `GlobalProjectID` (`"__global__"`, `workflow_reserved.go`) — a hidden project (kept out of `ProjectRepo.List`/`ProjectService.List`) that keeps the composite `(project_id, id)` keys + child FKs intact. It is also a **runnable** project (a scratch `root_path`) so it's the execution home for project-agnostic tools with no real project in scope (e.g. `deep_research` via the `mcp-external` proxy). UI surfaces global defs in run pickers but hides them from workflow management (run-only, admin-managed); `WorkflowExportService.Export` excludes them.

`EnsureGlobalDeepResearch(pool, clk, rootPath)` (`deep_research_seed.go` + `_data.go`) idempotently ensures the `__global__` project exists with a `root_path`, then create-if-absent seeds the bundled `deep-research` workflow (`is_global=1`, `callable_as_subworkflow=1`, result key `report`; 6 `cli_interactive` agents, finding schemas, L2 `quorum:2`) via direct SQL at startup — bypassing service-layer model validation since it's shipped data. Caller-context grounding is opt-in via the `mcp-external` `deep_research` `context` arg → `${EXTERNAL_CONTEXT}`. `EnsureGlobalDynamicWorkflow` (`dynamic_seed.go` + sibling `_data`/`_prompts_*`/`_planner`/`_schemas` files) seeds the bundled `dynamic` workflow: `callable_as_subworkflow=1`, a 10-def `fanout_template` catalog plus a workflow-local `node_role='planner'` def (`dynamic-planner`) and per-key `finding_schemas`. `agent_definitions.description` is required for `fanout_template` rows (the planner's selection text). Mutating `__global__` defs is admin-only — `api.denyNonAdminGlobalWrite`, `socket.denyGlobalWorkflowMutation` (plan revise/approve/cancel are runtime ops, deliberately exempt — [api/CLAUDE.md](../api/CLAUDE.md#plan-routes)).

## Constructor Pattern

Most service constructors take `(pool *db.Pool, clk clock.Clock)`. Pass `clock.Real()` in production; `clock.NewTest(fixedTime)` in tests.

**Exception:** `NewAgentDefinitionService(pool, clk, cliModelSvc, apiModelSvc, pythonScriptRepo)` additionally requires a `*CLIModelService` (validates `low_consumption_model` for cli_interactive) and `*APIModelService` (validates model and `low_consumption_model` for api mode). `NewSystemAgentDefinitionService(pool, clk, apiModelSvc)` also requires `*APIModelService` for api-mode model validation.

Workflow/agent definition CRUD routes: see [api/CLAUDE.md](../api/CLAUDE.md).
