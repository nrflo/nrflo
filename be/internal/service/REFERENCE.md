# Service Reference

Deep mechanics for this package. The auto-loaded map lives in [CLAUDE.md](CLAUDE.md); read this file when changing the plan lifecycle, global workflow seeds, or per-project env vars.

## Plan Lifecycle (`plan.go` + siblings)

Files: `plan.go`, `plan_revise.go`, `plan_manifest.go`, `plan_validate.go`/`plan_validate_refs.go`, `plan_schema.go`, `plan_templates.go`, `plan_limits.go`, `plan_materialize.go`, `plan_status.go`, `workflow_instance_nodes.go`.

- `PlanService`: draft/revise/approve→materialize/cancel/TTL-sweep; manifest v1 types + hash; `ValidatePlanManifest` returns one aggregated error.
- Template library resolution + availability recheck: `PlanTemplate` carries effective `reasoning_effort` plus a CLI type derived from the model provider; `EnabledTemplates`/`ValidateTemplatesEnabled` drop a template when its selected registry mode is unavailable/disabled, its CLI binary is absent (`CLIAvailable`), or api mode is off.
- Caps from config KV: `plan_max_layers`, `plan_max_nodes`, `plan_max_instruction_bytes`, `plan_draft_ttl_min` (project > global).
- `Materialize` writes an approved revision's nodes into `workflow_instance_nodes`/`workflow_instance_layer_policies` exactly once (hash-stamped conditional UPDATE — idempotent across continue/retry/restart).
- `DerivePlanInstanceStatus`/`IsPlanDriven`/`EffectivePhases` are the orchestrator's plan-boundary primitives — see [orchestrator/REFERENCE.md](../orchestrator/REFERENCE.md).
- Every `Revise` re-syncs an already plan-suspended instance's status (guarded: never clobbers a run executing static layers), so a poller sees `waiting_input` → `waiting_approval` once questions are answered.
- Reserved key `_workflow_plan` is server-owned and resolved ahead of `workflows.finding_schemas` in `FindingsService.Emit`; `findings_add`/socket `findings.add` reject `_`-prefixed reserved keys.

## Global Workflow Seeds

`EnsureGlobalDeepResearch(pool, clk, rootPath)` (`deep_research_seed.go` + `_data.go`) idempotently ensures the `__global__` project exists with a `root_path`, then create-if-absent seeds the bundled `deep-research` workflow (`is_global=1`, `callable_as_subworkflow=1`, result key `report`; 6 `cli_interactive` agents, finding schemas, L2 `quorum:2`) via direct SQL at startup — bypassing service-layer model validation since it is shipped data. Caller-context grounding is opt-in via the console `deep_research` tool's `context` arg (`internal/console/tools_deep_research.go`) → `${EXTERNAL_CONTEXT}`.

`EnsureGlobalDynamicWorkflow` (`dynamic_seed.go` + `_data`/`_prompts_*`/`_planner`/`_schemas.go`) seeds the bundled `dynamic` workflow: `callable_as_subworkflow=1`, a 10-def `fanout_template` catalog plus a workflow-local `node_role='planner'` def (`dynamic-planner`) and per-key `finding_schemas`. `agent_definitions.description` is required for `fanout_template` rows; `reasoning_effort` is optional.

Mutating `__global__` defs is admin-only — `api.denyNonAdminGlobalWrite`, `socket.denyGlobalWorkflowMutation` (plan revise/approve/cancel exempt — [api/CLAUDE.md](../api/CLAUDE.md#plan-routes)).

## Per-project env vars

Stored in `project_env_vars`. CRUD under `GET|PUT|DELETE /api/v1/projects/{id}/env-vars[/{name}]` (`handlers_project_env_vars.go`; writes admin-only). `ProjectEnvVarService` validates: name matches `^[A-Za-z_][A-Za-z0-9_]*$`, not in the reserved set (`NRFLO_PROJECT`, `NRFLO_AGENT_TOKEN`, `NRF_SESSION_ID`, `NRF_WORKFLOW_INSTANCE_ID`, `PATH`, `HOME`, …), value ≤ 4096 bytes. At workflow start `orchestrator.loadProjectEnv` fills `spawner.Config.ProjectEnv`; `prepareSpawn`/`prepareScriptSpawn` append them after nrflo-controlled vars for all backends.
