# manifest Package

Customer-facing manifest parsing, Python runtime, and init-customer scaffolding for api-mode workflows.

## Subpackages

| Package | Purpose |
|---------|---------|
| `config/` | Manifest parsing, tool validation, JSON Schema compilation |
| `python/` | Python script execution runtime (Runner, OSRunner, env scoping) |
| `scaffold/` | `init-customer` scaffolder (embedded customer-config-template tree) |

## config

`config.Load(dir string) (*Manifest, error)` reads `tool_manifest.yaml` from `dir` and returns a parsed `Manifest`. Each tool entry declares:
- `name` — tool name registered in the agent's tool registry
- `script` — relative path to the Python script within `dir`
- `description` — human-readable tool description
- `input_schema` — JSON Schema for tool input (compiled at load time via `jsonschema`)
- `config_files` — list of config file paths editable via the Config Editor
- `env_allow` — env var name patterns forwarded to the Python subprocess
- `review` — bool; when true, successful invocations create a `review_items` row

The `Manifest` is mtime-cached by the spawner (`loadManifestCached`) and reloaded only when `tool_manifest.yaml` changes on disk.

## python

`python.Runtime{Runner, ConfigDir}` executes a manifest tool's Python script:
- `Invoke(ctx, script, inputBytes, env, timeout)` — spawns `python3 <script>` with `inputBytes` on stdin; expects JSON on stdout
- `OSRunner` — production runner (wraps `os/exec`)
- `MatchEnv(patterns, environ)` — filters `os.Environ()` to the subset matching `env_allow` patterns

## scaffold

`scaffold.Scaffold(opts)` generates a new customer config directory from the embedded `customer-config-template/` tree:
- Template files use `{{.Name}}` substitution
- `--git` flag initializes a git repo in the output directory
- Called by `nrflo_server init-customer --out <dir> --name <Name>`

## Cross-references

- DB tables: `tool_dispatches`, `review_items`, `customer_config_versions` — see [be/internal/db/CLAUDE.md](../db/CLAUDE.md)
- Config editor service (versioned file editing): [be/internal/configeditor/CLAUDE.md](../configeditor/CLAUDE.md)
- Manifest tool dispatch + WS events: [be/internal/spawner/apirun/CLAUDE.md](../spawner/apirun/CLAUDE.md)
- Tool registry wiring: `be/internal/spawner/apirun/tools_manifest/`
