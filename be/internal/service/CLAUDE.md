# Service Package

Business logic layer separating domain logic from HTTP/socket handlers.

## File-to-Responsibility Mapping

| File | Domain |
|------|--------|
| `project.go` | Project CRUD operations |
| `ticket.go` | Ticket CRUD, close/reopen, search, `ValidateRunnable` (rejects blocked tickets) |
| `workflow.go` | Workflow operations (ticket + project scope): init, start/complete phase, state queries |
| `workflow_defs.go` (+ `_read`/`_validate`/`_agents`) | Workflow definitions CRUD (phases derived from agent_definitions at read time); validates `next_workflow_on_success` (no self-ref, must exist in same project, must be project-scoped) and `finalize_success`/`finalize_failure`/`pause_event` slots (same rules: `command` vs `script_id` mutually exclusive; any `script_id` must resolve to a `python_scripts` row of `kind='agent'`) |
| `workflow_config.go` | `BuildSpawnerConfig`: phases from agent_definitions (layer ASC, id ASC) |
| `workflow_types.go` | Type definitions: `WorkflowDef`, `PhaseDef`, `RestartDetail` |
| `layer_policy.go` | `ParseLayerPolicy`, `LayerPolicy.Required(denom)`, `ValidateLayerPolicy`; kinds: `any`, `all`, `quorum:N`, `percent:P` |
| `workflow_layer_policy.go` | `WorkflowLayerPolicyService`: Get/Set/Delete; validates quorum ≤ agent count |
| `workflow_validation.go` | `validateLayerConfig` (layer >= 0), `ValidateScopeType`, `ValidateGroups` |
| `workflow_response.go` | V4 response building: active agents, history, findings aggregation, phase status |
| `workflow_restart_details.go` | Restart detail loading: duration, context, message count from continued sessions |
| `agent.go` | Agent session operations; `Fail`/`Continue` return `(sessionID, error)` |
| `agent_definition.go` | Agent definition CRUD; validates `layer >= 0`; `execution_mode` cli_interactive/api/script (default cli_interactive); script requires `python_script_id` in same project and rejects binding to a `kind='tool'` row with `python_script_kind_mismatch`; validates `validation_commands` (max 20 entries, ≤1024 bytes each, no empty/whitespace), persisted as JSON-array string with default `"[]"`; `consultant=true` requires `execution_mode='api'` (effective values resolved against the current row on update). Consultant defs never become phases: the execution graph uses `repo.AgentDefinitionRepo.ListExecutable` (`List` + `consultant=0`), the v4 read model omits them, and layer-gap/pass-policy/quorum counts exclude them |
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
| `cli_model.go` | CLI model CRUD; `validateReasoningEffort` enforces allowed values (`xhigh` only for claude-opus-4-7/4-8); `normalizeFallbackModels` caps the claude-only `fallback_models` chain at 3; readonly rows only accept `reasoning_effort` + `fallback_models` updates |
| `api_model.go` | API model CRUD (provider: anthropic/openai); `validateAPIReasoningEffort` enforces `xhigh` only on Anthropic Opus 4.7/4.8; readonly rows only accept `reasoning_effort` updates; `IsValidModel` used by `agent_definition.go` and `system_agent_definition.go` for api-mode validation |
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
| `purge.go` | `PurgeService.PurgeInstanceTraceIfEnabled`: when an instance's `purge_on_completion` snapshot is set, redacts `agent_sessions` sensitive columns (kept as audit shells) and deletes its messages/findings/findings_history/artifacts(rows+files)/errors, then clears the instance's caller columns (`external_id`/`external_context`/`skip_tags`). One tx; artifact files first (best-effort). Audits `workflow.purged` (content-free counts) + broadcasts `EventWorkflowPurged`. Invoked from the orchestrator terminal hooks; the final result is delivered via the completion event, not retained. |
| `workflow_export.go` + `workflow_export_import.go` | `WorkflowExportService`: `Export` builds a v1.0 `types.WorkflowBundle` (strips project_id/workflow_id, dedupes referenced python scripts); `CheckImport` probes target project for workflow/script ID conflicts; `Import` applies bundle with action `overwrite` (cascade-delete then create), `rename` (`-N` suffix on conflicting workflow IDs), or `cancel` (no-op, `Skipped=true`). Python scripts always get freshly generated IDs; agent `PythonScriptID` is remapped before `CreateAgentDef`. |

## Per-project env vars

Stored in `project_env_vars` table (migration 000095). CRUD under `GET|PUT|DELETE /api/v1/projects/{id}/env-vars[/{name}]` (`handlers_project_env_vars.go`; writes admin-only). `ProjectEnvVarService` (`project_env_var.go`) validates: name matches `^[A-Za-z_][A-Za-z0-9_]*$`, not in reserved set (`NRFLO_PROJECT`, `NRFLO_AGENT_TOKEN`, `NRF_SESSION_ID`, `NRF_WORKFLOW_INSTANCE_ID`, `PATH`, `HOME`, etc.), value ≤ 4096 bytes. At workflow start, `orchestrator.loadProjectEnv` loads vars into `spawner.Config.ProjectEnv`; `prepareSpawn`/`prepareScriptSpawn` append them after nrflo-controlled vars for all backends (cli_interactive/api/script).

## Global workflows

A workflow definition is global when `workflows.is_global=1` (migration `000146`). Global defs are project-agnostic: `ListWorkflowDefs` (`workflow_defs_read_list.go`) unions them into **every** project's listing with **local-name precedence** (a project-local def shadows a same-named global one), so they are selectable/runnable from any project while execution stays project-scoped. `GetWorkflowDef` (`workflow_defs_read.go`), `loadFindingSchemas` (`finding_schema.go`), and the orchestrator's `resolveWorkflowDef` resolve a name under the selected project, then fall back to the global namespace.

Global defs are stored under the reserved `GlobalProjectID` (`"__global__"`, `workflow_reserved.go`) — an internal storage namespace that keeps the composite `(project_id, id)` keys + child FKs intact, **not** a user project: it is hidden from project listings (`ProjectRepo.List`/`ProjectService.List`) and `is_global` is the authoritative semantic. The UI surfaces global defs in run pickers (marked) but hides them from the workflow-management page (run-only, admin-managed). `WorkflowExportService.Export` excludes global defs (they aren't owned by the project).

`EnsureGlobalDeepResearch` (`deep_research_seed.go` + `_data.go`) idempotently seeds the `__global__` namespace row and the bundled `deep-research` workflow (`is_global=1`, 6 api agents, finding schemas, L2 `quorum:2`) via direct SQL at startup (`cli/serve.go`), bypassing the api-mode/model gates since it is data. Mutating `__global__` defs is admin-only — HTTP `api.denyNonAdminGlobalWrite`, socket `socket.denyGlobalWorkflowMutation`.

## Workflow Types

Key types in `workflow_types.go`:

- **`WorkflowDef`** — workflow definition with description, scope_type, groups, phases (derived from agent_definitions at read time)
- **`PhaseDef`** — phase definition with id, agent name, and layer number (built from agent_definitions)

## Constructor Pattern

Most service constructors take `(pool *db.Pool, clk clock.Clock)`. Pass `clock.Real()` in production; `clock.NewTest(fixedTime)` in tests.

**Exception:** `NewAgentDefinitionService(pool, clk, cliModelSvc, apiModelSvc, pythonScriptRepo)` additionally requires a `*CLIModelService` (validates `low_consumption_model` for cli_interactive) and `*APIModelService` (validates model and `low_consumption_model` for api mode). `NewSystemAgentDefinitionService(pool, clk, apiModelSvc)` also requires `*APIModelService` for api-mode model validation.

## Common Tasks

### Adding a New Agent Type

1. Create agent definition via API: `POST /api/v1/workflows/:wid/agents` with `layer` field
2. Update root `CLAUDE.md` agent references if user-visible

### Adding a New Workflow

1. Create workflow definition: `POST /api/v1/workflows`
2. Create agent definitions: `POST /api/v1/workflows/:wid/agents` with `layer` field
3. Update root `CLAUDE.md` Workflows table
