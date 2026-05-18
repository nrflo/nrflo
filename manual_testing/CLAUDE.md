# Manual integration testing harness

Per-provider Python harness that exercises the full path "real REST API → real DB → real spawner → real CLI binary → real agent CLI writes back via socket". Lives outside the Go test pyramid — each run spawns real CLI processes against real provider credentials.

## Hard rules

- **Manual launch only.** Not wired into any Makefile target, CI workflow, or pre-commit hook.
- **No Go test files or vitest test files** live here.
- **No Makefile changes** to expose it.

## Layout

```
manual_testing/
├── suite.md                 # canonical scenario catalogue (numbers + descriptions)
├── run_suite.py             # orchestrator: all provider folders run concurrently (per-folder NRFLO_HOME + NRFLO_SOCKET)
├── lib/                     # shared infra: api, db, runner, runtime, server, ws_client, http_mock, script_helpers, versions, credentials
├── engine/                  # provider-agnostic scenarios, run under the claude binary
├── claude/                  # claude-specific scenarios only (s05, s35)
├── codex/                   # codex-specific scenarios only (s05, s35)
├── gemini/                  # gemini-specific scenarios only (s05, s35)
├── opencode/                # opencode-specific scenarios only (s05, s27, s35)
├── python/                  # execution_mode='script' scenarios (no CLI, no LLM)
└── api/                     # execution_mode='api' scenarios (in-process Anthropic runner)
```

- `lib/runner.py` — `run_all(scenarios=…, provider=…, model=…, binary=…, mode=…, results_path=…)`
- `lib/runtime.py` — `Ctx` dataclass + `make_project` + `wait_for_workflow`
- `lib/server.py` — spawns `nrflo_server` on a fresh `NRFLO_HOME`
- `lib/versions.py` — probes `<binary> --version` for the capability matrix
- `<folder>/__init__.py` — explicit `ALL_SCENARIOS` list for that folder
- `<folder>/test.py` — entry point (`--parallel`, `--model`, `--only`, `--timeout`, `--results`)

Folder applicability is recorded in `suite.md` and verified by file presence in each folder. Cross-provider gates (`if ctx.provider == …`) are forbidden inside scenarios — divergent behaviour belongs in a per-provider folder.

## Concepts

- **Provider**: `engine`, `claude`, `codex`, `gemini`, `opencode`, `python`, or `api`. `engine` and the CLI providers run under `cli_interactive` (PTY relay) — `engine` uses the `claude` binary. `python` runs under `script` (execution_mode='script'). `api` runs the in-process Anthropic runner (`execution_mode='api'`); the runner SKIPs the whole folder when `lib/credentials.probe_oauth_token()` cannot resolve an OAuth bearer token from the macOS Keychain (service `Claude Code-credentials`) or `$ANTHROPIC_OAUTH_TOKEN`.
- **`Ctx`** (`lib/runtime.py:33`): carries server handle, REST client, provider, model, binary, mode, scenario label.
- **Scenario**: `run(ctx: Ctx) -> Result` where `Result = (name, "PASS"|"FAIL"|"SKIP", details)`. One function per file. Self-contained — no shared fixtures beyond `lib/runtime.py` helpers and `lib/script_helpers.py` for python scenarios.

## Runtime deps

Stdlib only, except `websockets` (required by `s37_ws_event_subscriber`).
Install via `pip install websockets` before running the CLI suites.

## How to run

```bash
make build

# full suite — all provider folders run concurrently with isolated NRFLO_HOME/NRFLO_SOCKET each; overwrites /capabilities.md
python3 manual_testing/run_suite.py

# subset
python3 manual_testing/run_suite.py --only=engine,python

# force one-at-a-time (debugging, rate-limited keys, etc.)
python3 manual_testing/run_suite.py --sequential

# single folder directly (useful for debugging)
python3 manual_testing/engine/test.py --only=s01 --parallel=1
python3 manual_testing/claude/test.py --only=s05 --parallel=1
python3 manual_testing/python/test.py --only=P01
python3 manual_testing/api/test.py --only=A01 --parallel=1
```

Each provider subprocess creates `/tmp/nrflo-manual-<provider>-<mode>-XXXX/` with the SQLite DB, per-scenario project roots, and `server.log`. The orchestrator collects results under `/tmp/nrflo-suite-<ts>/`. Directories are kept on exit.

Exit codes: `0` = all PASS/SKIP, `1` = any FAIL, `2` = fatal interruption.

`lib/server.py` gives each server its own `NRFLO_HOME` and `NRFLO_SOCKET` (short `/tmp/...` path; avoids macOS 104-byte AF_UNIX cap and stale-socket conflicts from prior crashes).

## Adding a new scenario

1. Pick the next free id in `suite.md` (`sNN` for CLI, `PNN` for python). Add a one-line description.
2. Default home is `engine/`. Create `engine/<id>_<short_name>.py` using `engine/s01_findings_save.py` (CLI) or `python/P01_findings_basic.py` (script) as the template. Do not branch on `ctx.provider` inside the file. Only put a scenario in a per-provider folder when the implementation must diverge per provider — in that case add the file to every applicable provider folder.
3. Append the module to that folder's `__init__.py::ALL_SCENARIOS`.
4. `python3 manual_testing/<folder>/test.py --only=<id> --parallel=1` to debug.
5. Run `python3 manual_testing/run_suite.py` once to regenerate `/capabilities.md`.

## Debugging

- Data dir (`/tmp/nrflo-manual-…`): open `nrflo.data`, query `agent_sessions` and `agent_messages` for the failing workflow instance.
- `server.log` in the same dir: search `ERROR` / `WARN` / `panic`.
- Suite log dir (`/tmp/nrflo-suite-…`): contains `<provider>.json` results and `<provider>.log` stdout.
