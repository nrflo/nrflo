# Service Package

Business logic layer separating domain logic from HTTP/socket handlers.

## File-to-Responsibility Mapping

| File | Domain |
|------|--------|
| `project.go` | Project CRUD operations |
| `ticket.go` | Ticket CRUD, close/reopen, search, `ValidateRunnable` (rejects closed/blocked tickets) |
| `workflow.go` | Workflow operations (ticket + project scope): init, start/complete phase, state queries |
| `workflow_defs.go` | Workflow definitions CRUD |
| `workflow_config.go` | Workflow config loading |
| `workflow_types.go` | Type definitions: `WorkflowDef`, `PhaseDef`, `RestartDetail` |
| `workflow_validation.go` | Validation: layer ordering, fan-in rules, project scope constraints |
| `workflow_response.go` | V4 response building: active agents, history, findings aggregation, phase status derivation from agent_sessions |
| `workflow_restart_details.go` | Restart detail loading: queries continued sessions for per-restart enrichment (duration, context, message count) |
| `agent.go` | Agent session operations; `Fail`/`Continue` return `(sessionID, error)` |
| `agent_definition.go` | Agent definition CRUD |
| `findings.go` | Findings add/append/get/delete operations |
| `chain.go` | Chain build, delete, dependency expansion, topological sort, cycle detection |
| `chain_preview.go` | Chain preview, custom order validation (validateCustomOrder, validateSameSet, computeDeps, PreviewChain) |
| `chain_append.go` | Append tickets to running chains |
| `daily_stats.go` | Daily stats computation from source tables |
| `git.go` | Git operations: paginated commit listing, commit detail with diff (os/exec, no DB) |
| `worktree.go` | Git worktree lifecycle: Setup (branch+worktree creation), MergeAndCleanup, Cleanup (os/exec, no DB) |
| `system_agent_definition.go` | System agent definition CRUD (global) |
| `default_template.go` | Default template CRUD (global, readonly enforcement) |
| `cli_model.go` | CLI model CRUD (global, readonly delete enforcement, enabled toggle with in-use check) |
| `global_settings.go` | Global and project-scoped settings key-value access (wraps `pool.GetConfig`/`SetConfig`/`GetProjectConfig`/`SetProjectConfig`) |
| `error_service.go` | Error tracking: `RecordError` (UUID gen, clock timestamp, DB insert, WS broadcast), `ListErrors` (paginated) |
| `snapshot.go` | WS snapshot provider (builds chunks from workflow state) |

## Workflow Types

Key types in `workflow_types.go`:

- **`WorkflowDef`** — workflow definition with ID, phases, description, scope_type
- **`PhaseDef`** — phase definition with agent name and layer number

## Constructor Pattern

Most service constructors take `(pool *db.Pool, clk clock.Clock)`. The clock is used for timestamps on DB records. In production, pass `clock.Real()`. In tests, pass `clock.NewTest(fixedTime)` for deterministic timestamps.

**Exception:** `NewAgentDefinitionService(pool, clk, cliModelSvc)` additionally requires a `*CLIModelService` for validating `low_consumption_model` against the `cli_models` DB table instead of a hardcoded model list.

## Common Tasks

### Adding a New Agent Type

1. Create agent definition via API: `POST /api/v1/workflows/:wid/agents` with model, timeout, and prompt template
2. Add to workflow phases via API: `PATCH /api/v1/workflows/:id` (or create a new workflow)
3. **Documentation updates:**
   - Root `CLAUDE.md` — update agent references

### Adding a New Workflow

1. Create workflow definition via API: `POST /api/v1/workflows` with phases and description
2. Ensure all referenced agents have definitions created via `POST /api/v1/workflows/:wid/agents`
3. **Documentation updates:**
   - Root `CLAUDE.md` — update Workflows table

### Modifying State Structure

1. Update state initialization in `workflow.go`
2. Update any code reading that state
3. **Documentation updates:**
   - Root `CLAUDE.md` — update state diagrams and State Storage section if user-visible
