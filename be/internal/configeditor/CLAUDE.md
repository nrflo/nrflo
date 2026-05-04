# configeditor Package

Versioned config file editing service and forward-only migration runner for api-mode customer config directories.

## Files

| File | Purpose |
|------|---------|
| `service.go` | `Service` — per-request config editor built from `customer_config_dir` project setting |
| `files.go` | File listing: merges manifest + tool `config_files` + on-disk yaml/json |
| `validate.go` | Schema validation helper (validates content against JSON Schema sidecar) |
| `migrate/runner.go` | Forward-only migration runner for customer config migrations |
| `migrate/registry.go` | Migration registry (maps version → migration func) |
| `migrate/deps.go` | Dependencies injected into migrations (uses `ConfigVersionRepo`) |
| `migrate/migrations/` | Versioned migration implementations |

## Service

`configeditor.Service` is constructed per HTTP request from the `customer_config_dir` project setting. It wraps a `*repo.ConfigVersionRepo` and a `manifest.Manifest`. Methods:

| Method | Description |
|--------|-------------|
| `ListFiles()` | Returns merged file list (manifest tools `config_files` + disk yaml/json) |
| `GetContent(file)` | Latest DB version if edited; falls back to disk file |
| `PutContent(file, content)` | Validates against sidecar schema, inserts new version row, returns version number |
| `GetHistory(file)` | All versions for the file, newest first |
| `Rollback(file, version)` | Creates a new version row with the content of the target version |

## DB

Config file versions are stored in the `customer_config_versions` table. Each write auto-increments the version number per `(project_id, file)` within a transaction. See [be/internal/db/CLAUDE.md](../db/CLAUDE.md) for the full schema.

## migrate

`migrate.Runner` applies forward-only migrations to a customer config directory. Migrations are keyed by integer version and registered in `registry.go`. Each migration receives a `Deps` struct with access to the `ConfigVersionRepo` for reading/writing versioned config files.

## WS Events

`PutContent` and `Rollback` broadcast `config_file.updated` (`ws.EventConfigFileUpdated`) after a successful write. See [be/internal/ws/CLAUDE.md](../ws/CLAUDE.md).
