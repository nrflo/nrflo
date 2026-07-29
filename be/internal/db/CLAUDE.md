# Database Package

SQLite database layer with connection pooling, auto-migration, and embedded SQL migration files.

## Querier Interface

`db.go` exports a `Querier` interface satisfied by both `*DB` and `*Pool`:
- Methods: `Exec`, `Query`, `QueryRow`, `Begin`
- Repos that don't need pool/DB-specific features accept `db.Querier`
- Enables passing either `*DB` or `*Pool` to the same repo constructor

## Connection Pool & Write Concurrency

`pool.go` manages the connection pool:
- Max connections: 10
- Max idle connections: 5
- Pure Go SQLite via `modernc.org/sqlite` (no CGO)

The DSN (`buildDSN`, db.go) sets per-connection `busy_timeout(10000)`, `foreign_keys(1)`, and `_txlock=immediate`; WAL is enabled at open. `_txlock=immediate` makes every `Begin()` issue BEGIN IMMEDIATE, so writers queue on busy_timeout instead of failing deferred read→write upgrades with SQLITE_BUSY_SNAPSHOT (which bypasses busy_timeout by design). Wrap multi-statement write transactions in `db.WithBusyRetry` (retry.go) as a contention backstop; each attempt must open its own transaction.

## Schema and Migrations

Schema is defined by the migration files. List them with `ls be/internal/db/migrations/`; each file is the source of truth for its table.

Migrations are forward-only SQL files in `migrations/`, embedded via `//go:embed *.sql` in `migrations/embed.go`. They run automatically on server startup via golang-migrate. To add a migration: create `migrations/NNNNNN_description.up.sql` (next sequence number). Down migrations are not used — rollbacks are done via new forward migrations.

The clock abstraction (`internal/clock`) drives all `created_at`/`updated_at` timestamp writes in repo constructors; pass `clock.Real()` in production and `clock.NewTest(t)` in tests.

Foreign keys use `ON DELETE CASCADE` for child rows tied to a parent (e.g., agent_sessions → workflow_instances; workflow_instances → workflows via `(def_project_id, workflow_id)` and → projects via `project_id`, migration `000165` — def semantics in [service/CLAUDE.md](../service/CLAUDE.md#global-workflows)). See the migration files for per-table FK details.

`agent_sessions.node_id` is execution identity (which slot in the run — session dedupe, retry target, callback scope, trace lane layering); `agent_sessions.agent_type` stays template identity (which `agent_definitions` row — model/tag/prompt resolution). They are equal for every static workflow today. `agent_definitions.node_role` (`static`|`planner`|`fanout_template`) marks defs that must never auto-execute as a phase, alongside `consultant`.

The `models` table has one row per provider/model pair; non-empty `cli_model` and `api_model` columns enable each mode, with separate context windows and JSON effort lists. Agent definitions and historical run/session model references use its canonical slug IDs. A nullable `release_date` (ISO, NULL=unknown) drives the console picker's newest-release-first ordering (`service.SortModelsForPicker`). `models.provider` carries no CHECK constraint (dropped in migration `000193`'s rebuild): rows may reference any name in `custom_providers` (migration `000192`: `name` PK, `base_url`, optional `api_key`, `api_wire` CHECK IN `('responses','chat_completions')`, `enabled`), validated at the service layer via `service.resolveProvider`, not a DB constraint.

## Per-project & global settings (config table)

Composite PK `(project_id, key)`. `project_id=''` is the sentinel for global (non-project) settings. Accessors at `be/internal/db/pool.go:94-130`:

- `pool.GetConfig(key)` / `pool.SetConfig(key, value)` — global KV
- `pool.GetProjectConfig(projectID, key)` / `pool.SetProjectConfig(projectID, key, value)` — project-scoped KV

This is the canonical KV store — do not create new tables for similar use cases. Plan caps (`plan_max_layers`/`plan_max_nodes`/`plan_max_instruction_bytes`/`plan_max_questions`/`plan_draft_ttl_min`) live here too — see `service/plan_limits.go`.

## Plan tables

`plan_revisions` (migration `000158`) is append-only: PK `(instance_id, revision)`, `author` CHECK IN `('planner','caller')`, FK → `workflow_instances` ON DELETE CASCADE — a revision row is never UPDATEd. `workflow_plans` is the mutable head: PK `instance_id`, `status` CHECK IN `('draft','approved','cancelled')`, `latest_revision`/`approved_revision`/`goal`, plus the materialization stamp `materialized_revision`/`materialized_hash` (migration `000159`, default `0`/`''`). See `repo/plan.go` + `service/plan.go`.

`workflow_instance_nodes` and `workflow_instance_layer_policies` (migration `000159`) are instance-scoped: an approved plan revision's nodes/layer policies are written into them exactly once by `service.PlanService.Materialize`, keyed on `instance_id`, FK CASCADE. `workflow_instance_nodes` is INSERT-only (immutable once materialized — no update/delete API on `repo.InstanceNodeRepo`); `node_id`/`layer`/`agent_type`/`instructions` mirror a plan manifest node, with `layer` offset above the workflow definition's static executable layers. `workflow_instances.status` (migration `000136`'s CHECK, rebuilt in `000159`, narrowed in `000211`) also accepts `planning`/`waiting_input`/`waiting_approval` — the plan-boundary suspend statuses (see `orchestrator/CLAUDE.md`), distinct from `waiting` (pause_after).

`agent_definitions.prompt_mode` (`full`|`stepwise`, migration `000202`) gates the nullable `steps` JSON column (a `model.StepDefinition` array); `agent_step_cursors` tracks a stepwise agent's progress through that array, PK `(workflow_instance_id, node_id)` with FK CASCADE to `workflow_instances` — keyed by instance+node rather than session so the cursor survives session restarts/rotations within the same node. Its `rejections` column (migration `000204`) is a JSON map `step_id -> int`, a durable per-step rejection counter that survives session rotation/retry. Migration `000205` adds `default_templates.steps` (seed-only step JSON read by `ProjectService.seedTieredWorkflows`) and converts the seeded layer-0 `setup-analyzer` role plus the live `nrworkflow`/`feature`/`plan` def to `prompt_mode='stepwise'`. `delegations` (migration `000216`) is a durable, never-deleted log of delegate fanout calls (caller session, depth, per-worker session ids, terminal status, consumed-once marker) — like `refinery_runs` it carries no foreign keys so rows outlive cascade-deleted sessions/instances. `consults` (migration `000217`) mirrors it for `Spawner.Consult`/`ConsultHost` calls (caller session, consultant id, single child session id, terminal status) — consult children have no caller column of their own on `agent_sessions`. `workflow_instances.origin`/`origin_session_id` (migration `000218`) record the launch surface (`"console"`/`"human"`, empty = unknown/pre-existing) and, for console starts, the launching console session id.

## Files

| File | Purpose |
|------|---------|
| `db.go` | SQLite connection setup, `Querier` interface |
| `pool.go` | Connection pool (10 max, 5 idle) |
| `retry.go` | `IsBusy` + `WithBusyRetry` for write transactions |
| `migrate.go` | Migration runner |
| `migrations/` | SQL files (embedded via `//go:embed`) |
| `migrations/embed.go` | Go embed directive |
