# Service Package

Business logic layer separating domain logic from HTTP/socket handlers.

## File-to-Responsibility Mapping

| File | Domain |
|------|--------|
| `project.go` | Project CRUD operations |
| `ticket.go` | Ticket CRUD, close/reopen, search, `ValidateRunnable` (rejects blocked tickets) |
| `workflow.go` | Workflow operations (ticket + project scope): init, start/complete phase, state queries |
| `workflow_defs.go` | Workflow definitions CRUD (phases derived from agent_definitions at read time); validates `next_workflow_on_success` (no self-ref, must exist in same project, must be project-scoped) |
| `workflow_config.go` | `BuildSpawnerConfig`/`BuildSpawnerConfigWithPolicies`: phases from agent_definitions (layer ASC, id ASC) + LayerPolicies |
| `workflow_types.go` | Type definitions: `WorkflowDef`, `PhaseDef`, `RestartDetail` |
| `layer_policy.go` | `ParseLayerPolicy`, `LayerPolicy.Required(denom)`, `ValidateLayerPolicy`; kinds: `any`, `all`, `quorum:N`, `percent:P` |
| `workflow_layer_policy.go` | `WorkflowLayerPolicyService`: Get/Set/Delete; validates quorum ≤ agent count |
| `workflow_validation.go` | `validateLayerConfig` (layer >= 0), `ValidateProjectScope`, `ValidateScopeType`, `ValidateGroups` |
| `workflow_response.go` | V4 response building: active agents, history, findings aggregation, phase status |
| `workflow_restart_details.go` | Restart detail loading: duration, context, message count from continued sessions |
| `agent.go` | Agent session operations; `Fail`/`Continue` return `(sessionID, error)` |
| `agent_definition.go` | Agent definition CRUD; validates `layer >= 0`; `execution_mode` cli_interactive/api/script (default cli_interactive); script requires `python_script_id` in same project |
| `findings.go` | Findings add/append/get/delete |
| `chain.go` | Chain build, dependency expansion, topological sort, cycle detection |
| `chain_preview.go` | `PreviewChain`, custom order validation |
| `chain_append.go` | Append tickets to running chains |
| `chain_remove.go` | Remove pending items from running chains |
| `daily_stats.go` | Daily stats computation |
| `git.go` | Paginated commit listing and commit detail (os/exec) |
| `worktree.go` | Git worktree lifecycle: Setup, MergeAndCleanup, Cleanup |
| `system_agent_definition.go` | System agent definition CRUD (global) |
| `default_template.go` | Default template CRUD (global, readonly enforcement) |
| `cli_model.go` | CLI model CRUD; `validateReasoningEffort` enforces allowed values (`xhigh` only for claude-opus-4-7); readonly rows only accept `reasoning_effort` updates |
| `global_settings.go` | Key-value settings (wraps `pool.GetConfig`/`SetConfig`/`GetProjectConfig`/`SetProjectConfig`) |
| `claude_limits.go` | `ClaudeLimitsService`: typed facade for Claude rate-limit state; `Update` applies a per-window monotonic guard (rejects pct decreases within an active reset window, epsilon 0.5) and returns `UpdateResult{Changed}` so callers can suppress no-op broadcasts. |
| `error_service.go` | `RecordError` (UUID, clock, DB insert, WS broadcast), `ListErrors` (paginated) |
| `notification.go` | Notification channel CRUD + secret masking + TestSend + ListDeliveries |
| `snapshot.go` | WS snapshot provider |
| `insights.go` | `Summary`/`EditRate`/`Throughput`; `ParseRange` (7d/30d), `ParseBucket` (1h/6h/1d) |
| `workflow_chain.go` | `WorkflowChainService`: chain+step CRUD; validates dense positions, step 0 project-scope, workflow_name resolves |
| `workflow_chain_run.go` | `WorkflowChainRunService`: CreateRun, GetRunDetail, ListRuns, SetNextStepInstructions, SetNextStepTicket |
| `python_script.go` | `PythonScriptService`: Create/Get/List/Update/Delete; validates `file_path` (absolute, exists, `.py`) |
| `user_service.go` | `UserService.Delete`: self-delete → system-user → last-admin checks; system users flagged via `users.system=1` |
| `python_script_validate.go` | `Validate(ctx, code)`: runs `python3` AST parse with 5s timeout; degrades gracefully if python3 absent |
| `project_env_var.go` | `ProjectEnvVarService`: List/Upsert/Delete; validates name regex, reserved names, 4096-byte value cap |

## Per-project env vars

Stored in `project_env_vars` table (migration 000095). CRUD under `GET|PUT|DELETE /api/v1/projects/{id}/env-vars[/{name}]` (`handlers_project_env_vars.go`; writes admin-only). `ProjectEnvVarService` (`project_env_var.go`) validates: name matches `^[A-Za-z_][A-Za-z0-9_]*$`, not in reserved set (`NRFLO_PROJECT`, `NRFLO_AGENT_TOKEN`, `NRF_SESSION_ID`, `NRF_WORKFLOW_INSTANCE_ID`, `PATH`, `HOME`, etc.), value ≤ 4096 bytes. At workflow start, `orchestrator.loadProjectEnv` loads vars into `spawner.Config.ProjectEnv`; `prepareSpawn`/`prepareScriptSpawn` append them after nrflo-controlled vars for all backends (cli/api/script), and `tools_manifest.New` forwards them to manifest tool dispatch.

## Workflow Types

Key types in `workflow_types.go`:

- **`WorkflowDef`** — workflow definition with description, scope_type, groups, phases (derived from agent_definitions at read time)
- **`PhaseDef`** — phase definition with id, agent name, and layer number (built from agent_definitions)

## Constructor Pattern

Most service constructors take `(pool *db.Pool, clk clock.Clock)`. Pass `clock.Real()` in production; `clock.NewTest(fixedTime)` in tests.

**Exception:** `NewAgentDefinitionService(pool, clk, cliModelSvc)` additionally requires a `*CLIModelService` for validating `low_consumption_model`.

## Common Tasks

### Adding a New Agent Type

1. Create agent definition via API: `POST /api/v1/workflows/:wid/agents` with `layer` field
2. Update root `CLAUDE.md` agent references if user-visible

### Adding a New Workflow

1. Create workflow definition: `POST /api/v1/workflows`
2. Create agent definitions: `POST /api/v1/workflows/:wid/agents` with `layer` field
3. Update root `CLAUDE.md` Workflows table
