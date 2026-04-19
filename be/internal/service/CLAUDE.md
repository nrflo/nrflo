# Service Package

Business logic layer separating domain logic from HTTP/socket handlers.

## File-to-Responsibility Mapping

| File | Domain |
|------|--------|
| `project.go` | Project CRUD operations |
| `ticket.go` | Ticket CRUD, close/reopen, search, `ValidateRunnable` (rejects closed/blocked tickets) |
| `workflow.go` | Workflow operations (ticket + project scope): init, start/complete phase, state queries |
| `workflow_defs.go` | Workflow definitions CRUD (no phases in create/update; phases derived from agent_definitions at read time) |
| `workflow_config.go` | Workflow config loading: `BuildSpawnerConfig` and `parseWorkflowDefFromDB` derive phases from agent_definitions (layer field), sorted by layer ASC, id ASC |
| `workflow_types.go` | Type definitions: `WorkflowDef`, `PhaseDef`, `RestartDetail` |
| `workflow_validation.go` | Validation: `validateLayerConfig` (layer ordering, fan-in rules), `ValidateProjectScope`, `ValidateScopeType`, `ValidateGroups` |
| `workflow_response.go` | V4 response building: active agents, history, findings aggregation, phase status derivation from agent_sessions |
| `workflow_restart_details.go` | Restart detail loading: queries continued sessions for per-restart enrichment (duration, context, message count) |
| `agent.go` | Agent session operations; `Fail`/`Continue` return `(sessionID, error)` |
| `agent_definition.go` | Agent definition CRUD (includes `layer` field; create/update validates fan-in rules via `validateLayerConfigForWorkflow` in `agent_definition_helpers.go`) |
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
| `snapshot.go` | WS snapshot provider (builds chunks from workflow state) |

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
