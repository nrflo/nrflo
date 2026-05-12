# Database Package

SQLite database layer with connection pooling, auto-migration, and embedded SQL migration files.

## Querier Interface

`db.go` exports a `Querier` interface satisfied by both `*DB` and `*Pool`:
- Methods: `Exec`, `Query`, `QueryRow`, `Begin`
- Repos that don't need pool/DB-specific features accept `db.Querier`
- Enables passing either `*DB` or `*Pool` to the same repo constructor

## Connection Pool

`pool.go` manages the connection pool:
- Max connections: 10
- Max idle connections: 5
- Pure Go SQLite via `modernc.org/sqlite` (no CGO)

## Schema and Migrations

Schema is defined by the migration files. List them with `ls be/internal/db/migrations/`; each file is the source of truth for its table.

Migrations are forward-only SQL files in `migrations/`, embedded via `//go:embed *.sql` in `migrations/embed.go`. They run automatically on server startup via golang-migrate. To add a migration: create `migrations/NNNNNN_description.up.sql` (next sequence number). Down migrations are not used — rollbacks are done via new forward migrations.

The clock abstraction (`internal/clock`) drives all `created_at`/`updated_at` timestamp writes in repo constructors; pass `clock.Real()` in production and `clock.NewTest(t)` in tests.

Foreign keys use `ON DELETE CASCADE` for child rows tied to a parent (e.g., agent_sessions → workflow_instances, workflow_instances → tickets). See the migration files for per-table FK details.

## Files

| File | Purpose |
|------|---------|
| `db.go` | SQLite connection setup, `Querier` interface |
| `pool.go` | Connection pool (10 max, 5 idle) |
| `migrate.go` | Migration runner |
| `migrations/` | SQL files (embedded via `//go:embed`) |
| `migrations/embed.go` | Go embed directive |
