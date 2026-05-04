# be/internal/nrvapp

Package `nrvapp` provides customer config management for api-mode workflows.
It comprises four sub-packages plus a scaffold tool.

## Sub-packages

| Package | Purpose |
|---------|---------|
| `config/` | Manifest parsing, validation, JSON Schema compilation |
| `python/` | Python script execution runtime |
| `configeditor/` | Versioned config file editing service (DB-backed) |
| `configmigrate/` | Forward-only config migration runner |
| `scaffold/` | `init-customer` scaffolder (embedded template tree) |

---

## config/

### Manifest format (`tool_manifest.yaml`)

```yaml
tools:
  - name: lookup_sku
    type: python_script
    description: Look up product details by SKU
    script: tools/lookup_sku.py
    input_schema:
      type: object
      properties:
        sku: { type: string }
      required: [sku]
    config_files:
      - path: catalog.yaml
        schema_path: catalog.schema.json   # optional sidecar
```

**Supported types**: `python_script` only. Types `builtin` and `config_template` are rejected with an explicit error.

All `script` and `config_file` paths must be relative (no `..` traversal).
`input_schema` is required for every tool; compiled once by `config.Load()` using Draft2020.

### Key types

- `Manifest.Tool(name) (*Tool, bool)` — O(1) lookup by name
- `Manifest.ValidateInput(toolName, input) error` — validate JSON-compatible input against input_schema
- `Load(dir string) (*Manifest, error)` — reads `dir/tool_manifest.yaml`

---

## python/

### Runtime contract

- **stdin**: JSON object (validated by caller before invocation)
- **stdout**: JSON object (tool output)
- **non-zero exit**: → `ScriptError{ExitCode, Stderr, Cause}`
- **stderr**: last 4 KB captured in `ScriptError.Stderr`
- **timeout**: per-call `time.Duration`; on expiry the entire process group receives SIGKILL

### Security

- `OSRunner` sets `SysProcAttr.Setpgid = true` so child processes are in a new process group; SIGKILL targets the group on timeout.
- `env_allow` patterns (glob via `filepath.Match`) scope the environment: only matching keys pass through.
- `resolveScript` rejects empty, absolute, and `..`-traversal script paths.
- No real python3 is required by tests — `OSRunner` accepts an injectable `cmdFactory`.

### Key types

- `Runner` interface: `Invoke(ctx, scriptPath, input, env, timeout) ([]byte, error)`
- `OSRunner` (implements `Runner`): real `python3` from PATH; `NewOSRunner()`
- `Runtime`: wraps a Runner + configDir, resolves relative script paths
- `ScriptError`: non-zero exit error with ExitCode, Stderr, Cause
- `MatchEnv(patterns, environ) []string`: filter env vars by glob patterns
- `FilterOSEnv(patterns) []string`: same but reads `os.Environ()`

---

## configeditor/

### DB-first semantics

- `Get(projectID, file)`: returns DB content if `LatestVersion > 0`, otherwise reads from disk.
- `Put(projectID, file, actor, content)`: validates against sidecar `*.schema.json`, then inserts via `NrvappConfigVersionRepo`.
- `History(projectID, file)`: newest-first version list.
- `Rollback(projectID, file, actor, toVersion)`: inserts a new version with old content (append-only).

### File enumeration (`List`)

Returns `[]FileMeta` for:
1. `tool_manifest.yaml` itself
2. `ConfigFiles` entries from each manifest tool
3. Sibling `*.yaml` / `*.yml` / `*.json` files in `configDir`

`FileMeta.SchemaPath` is auto-detected from a `<name>.schema.json` sidecar if not explicit.

---

## configmigrate/

### Registry

Call `Register(version int, name string, fn MigrationFn)` (usually in `init()`) to add a migration.
Panics on duplicate version, zero/negative version, or nil function.

### Runner

`Run(ctx, dir, deps Deps) error` applies all registered migrations ahead of the stored pointer, in ascending version order. The pointer is persisted in `NrvappConfigVersionRepo` under the sentinel file `__configmigrate__`.

### Deps

Provides utilities to migration functions:
- `Deps.Dir() string` — config directory path
- `Deps.Backup(ctx, file) error` — snapshot a file into the DB before mutation
- `Deps.Validate(file, schemaBytes) error` — YAML→JSON Schema validation

Create via `NewDeps(dir, projectID, repo, clk)`.

---

## scaffold/

`nrflo_server init-customer --out <dir> [--name <name>] [--force] [--git]`

Materializes `customer-config-template/` into `--out`, applying `{{Name}}` substitution to text files.
Text extensions: `.yaml`, `.yml`, `.json`, `.md`, `.txt`, `.py`, `.sh`, `.toml`, `.cfg`, `.ini`.
Python scripts (`*.py`) are written with mode `0755`; all others `0644`.
`--force` allows overwriting a non-empty target directory.
`--git` runs `git init && git add . && git commit` after materialization (best-effort).

The embedded template contains only `python_script` tools (no `builtin`/`config_template` entries).
