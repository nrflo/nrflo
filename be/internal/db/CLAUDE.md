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

Foreign keys use `ON DELETE CASCADE` for child rows tied to a parent (e.g., agent_sessions → workflow_instances, workflow_instances → tickets). See the migration files for per-table FK details.

`agent_sessions.node_id` is execution identity (which slot in the run — session dedupe, retry target, callback scope, trace lane layering); `agent_sessions.agent_type` stays template identity (which `agent_definitions` row — model/tag/prompt resolution). They are equal for every static workflow today. `agent_definitions.node_role` (`static`|`planner`|`fanout_template`) marks defs that must never auto-execute as a phase, alongside `consultant`.

## Per-project & global settings (config table)

Composite PK `(project_id, key)`. `project_id=''` is the sentinel for global (non-project) settings. Accessors at `be/internal/db/pool.go:94-130`:

- `pool.GetConfig(key)` / `pool.SetConfig(key, value)` — global KV
- `pool.GetProjectConfig(projectID, key)` / `pool.SetProjectConfig(projectID, key, value)` — project-scoped KV

This is the canonical KV store — do not create new tables for similar use cases. Plan caps (`plan_max_layers`/`plan_max_nodes`/`plan_max_instruction_bytes`/`plan_max_questions`/`plan_draft_ttl_min`) live here too — see `service/plan_limits.go`.

## Plan tables

`plan_revisions` (migration `000158`) is append-only: PK `(instance_id, revision)`, `author` CHECK IN `('planner','caller')`, FK → `workflow_instances` ON DELETE CASCADE — a revision row is never UPDATEd. `workflow_plans` is the mutable head: PK `instance_id`, `status` CHECK IN `('draft','approved','cancelled')`, `latest_revision`/`approved_revision`/`goal`. Kept separate from `workflow_instances.status` (whose CHECK, migration `000136`, would need a full table rebuild to extend) — "plan ready" is derived from the head row. See `repo/plan.go` + `service/plan.go`.

## Files

| File | Purpose |
|------|---------|
| `db.go` | SQLite connection setup, `Querier` interface |
| `pool.go` | Connection pool (10 max, 5 idle) |
| `retry.go` | `IsBusy` + `WithBusyRetry` for write transactions |
| `migrate.go` | Migration runner |
| `migrations/` | SQL files (embedded via `//go:embed`) |
| `migrations/embed.go` | Go embed directive |
