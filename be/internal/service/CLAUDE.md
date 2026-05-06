# Service Package

Business logic layer separating domain logic from HTTP/socket handlers.

## File-to-Responsibility Mapping

| File | Domain |
|------|--------|
| `project.go` | Project CRUD operations |
| `ticket.go` | Ticket CRUD, close/reopen, search, `ValidateRunnable` (rejects blocked tickets) |
| `workflow.go` | Workflow operations (ticket + project scope): init, start/complete phase, state queries |
| `workflow_defs.go` | Workflow definitions CRUD (no phases in create/update; phases derived from agent_definitions at read time) |
| `workflow_config.go` | Workflow config loading: `BuildSpawnerConfig` and `parseWorkflowDefFromDB` derive phases from agent_definitions (layer field), sorted by layer ASC, id ASC. `BuildSpawnerConfigWithPolicies` wraps `BuildSpawnerConfig` and attaches `LayerPolicies` per workflow. |
| `workflow_types.go` | Type definitions: `WorkflowDef`, `PhaseDef`, `RestartDetail` |
| `layer_policy.go` | Parser/validator for pass_policy strings: `ParseLayerPolicy`, `LayerPolicy.Required(denom)`, `LayerPolicy.String()`, `ValidateLayerPolicy(s, agentCount)`. Supported kinds: `any`, `all`, `quorum:N`, `percent:P`. |
| `workflow_layer_policy.go` | `WorkflowLayerPolicyService`: `GetLayerPolicies(projectID, workflowID)` → `map[int]string`; `SetLayerPolicy` (counts agents, validates, upserts); `DeleteLayerPolicy`. Constructor: `NewWorkflowLayerPolicyService(pool, clk)`. |
| `workflow_validation.go` | Validation: `validateLayerConfig` (layer >= 0 bound), `ValidateProjectScope`, `ValidateScopeType`, `ValidateGroups` |
| `workflow_response.go` | V4 response building: active agents, history, findings aggregation, phase status derivation from agent_sessions |
| `workflow_restart_details.go` | Restart detail loading: queries continued sessions for per-restart enrichment (duration, context, message count) |
| `agent.go` | Agent session operations; `Fail`/`Continue` return `(sessionID, error)` |
| `agent_definition.go` | Agent definition CRUD (includes `layer` field; create/update validates layer >= 0 via `validateLayerConfigForWorkflow` in `agent_definition_helpers.go`). `execution_mode` accepts `"cli"` (default), `"api"`, or `"script"`. When `execution_mode="script"`, `python_script_id` must be non-empty and must reference an existing row in `python_scripts` belonging to the same project. |
| `findings.go` | Findings add/append/get/delete operations |
| `chain.go` | Chain build, delete, dependency expansion, topological sort, cycle detection |
| `chain_preview.go` | Chain preview, custom order validation (validateCustomOrder, validateSameSet, computeDeps, PreviewChain) |
| `chain_append.go` | Append tickets to running chains |
| `chain_remove.go` | Remove pending items from running chains |
| `daily_stats.go` | Daily stats computation from source tables |
| `git.go` | Git operations: paginated commit listing, commit detail with diff (os/exec, no DB) |
| `worktree.go` | Git worktree lifecycle: Setup (branch+worktree creation), MergeAndCleanup, Cleanup (os/exec, no DB) |
| `system_agent_definition.go` | System agent definition CRUD (global) |
| `default_template.go` | Default template CRUD (global, readonly enforcement) |
| `cli_model.go` | CLI model CRUD (global, readonly delete enforcement, enabled toggle with in-use check). `validateReasoningEffort(cliType, mappedModel, effort)` enforces allowed values (`""`, `low`, `medium`, `high`, `xhigh`, `max`); `xhigh` is only accepted when `cliType == "claude"` and `mappedModel` starts with `claude-opus-4-7` (covers both 200k and 1M variants). Called from `Create` and `Update` (Update overlays the provided fields on top of the existing row before validating). On `read_only` cli_models rows, `Update` only accepts changes to `reasoning_effort`; any non-nil `display_name`, `mapped_model`, `context_length`, or `enabled` in the request returns `only reasoning_effort can be updated on built-in models` (mapped to HTTP 400 by the handler). |
| `global_settings.go` | Global and project-scoped settings key-value access (wraps `pool.GetConfig`/`SetConfig`/`GetProjectConfig`/`SetProjectConfig`) |
| `error_service.go` | Error tracking: `RecordError` (UUID gen, clock timestamp, DB insert, WS broadcast), `ListErrors` (paginated) |
| `notification.go` | Notification channel CRUD + secret masking/unmasking + PATCH mask-preserves semantics + TestSend (enqueue synthetic delivery) + ListDeliveries |
| `snapshot.go` | WS snapshot provider (builds chunks from workflow state) |
| `insights.go` | Insights service for review/dispatch dashboards: `Summary`, `EditRate`, `Throughput`; wraps `DispatchRepo` and `ReviewRepo`. Helpers: `ParseRange` (7d/30d → time.Time), `ParseBucket` (1h/6h/1d → time.Duration). |
| `workflow_chain.go` | WorkflowChainService: CreateChain/GetChain/ListChains/UpdateChain/DeleteChain + AppendStep/UpdateStep/DeleteStep/ReorderSteps. Validates dense step positions, step 0 project-scope, workflow_name resolves via WorkflowService.GetWorkflowDef, require_ticket_handoff only when scope_type=ticket. Empty step lists are accepted at create time and rejected at run time by CreateRun. Constructor: `NewWorkflowChainService(pool, clk, wfSvc)`. |
| `workflow_chain_run.go` | WorkflowChainRunService: CreateRun (create run + materialize steps), GetRunDetail (run + steps), ListRuns (by project/chain), SetNextStepInstructions (update next pending step's instructions_used from instance_id), SetNextStepTicket (update next pending step's ticket_id from instance_id). Constructor: `NewWorkflowChainRunService(pool, clk)`. |
| `python_script.go` | PythonScriptService: Create (auto-generates `ps-xxxxxx` ID, requires name), Get/List/Update/Delete scoped by projectID. Constructor: `NewPythonScriptService(pool, clk)`. Wraps `repo.PythonScriptRepo`. |
| `user_service.go` | `UserService.Delete` check order: self-delete (`ErrSelfDelete`) → system user (`ErrSystemUser`) → last-admin (`ErrLastAdmin`). `ErrSystemUser` is a plain `errors.New` sentinel; system users are flagged via `users.system=1` (migration 000086; only `usr_admin_seed` is flagged). No API path to set/clear this flag — migration-managed only. |
| `python_script_validate.go` | PythonScriptValidator: `Validate(ctx, code) ValidationResult{OK,Error,Line,Col}`. Writes code to `/tmp/nrflo/validate-*.py`, runs `python3 -c <ast-parse-script>` with 5s timeout. Gracefully degrades to `{OK:true}` when `python3` is not in PATH. `lookPath`/`cmdFactory` are injectable for testing. |

## Workflow Types

Key types in `workflow_types.go`:

- **`WorkflowDef`** — workflow definition with description, scope_type, groups, phases (derived from agent_definitions at read time)
- **`PhaseDef`** — phase definition with id, agent name, and layer number (built from agent_definitions)

## Constructor Pattern

Most service constructors take `(pool *db.Pool, clk clock.Clock)`. The clock is used for timestamps on DB records. In production, pass `clock.Real()`. In tests, pass `clock.NewTest(fixedTime)` for deterministic timestamps.

**Exception:** `NewAgentDefinitionService(pool, clk, cliModelSvc)` additionally requires a `*CLIModelService` for validating `low_consumption_model` against the `cli_models` DB table instead of a hardcoded model list.

## Common Tasks

### Adding a New Agent Type

1. Create agent definition via API: `POST /api/v1/workflows/:wid/agents` with model, timeout, prompt template, and `layer` (determines phase execution order)
3. **Documentation updates:**
   - Root `CLAUDE.md` — update agent references

### Adding a New Workflow

1. Create workflow definition via API: `POST /api/v1/workflows` with description and scope_type
2. Create agent definitions via `POST /api/v1/workflows/:wid/agents` with `layer` field to define phase execution order
3. **Documentation updates:**
   - Root `CLAUDE.md` — update Workflows table

### Modifying State Structure

1. Update state initialization in `workflow.go`
2. Update any code reading that state
3. **Documentation updates:**
   - Root `CLAUDE.md` — update state diagrams and State Storage section if user-visible
