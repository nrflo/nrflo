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
├── run_suite.py             # cross-provider orchestrator (5 providers parallel, scenarios sequential)
├── lib/                     # shared infra: api, db, runner, runtime, server, ws_client, http_mock, script_helpers, versions
├── claude/                  # per-provider scenarios, __init__.py, test.py
├── codex/
├── gemini/
├── opencode/
└── python/                  # execution_mode='script' scenarios (no CLI, no LLM)
```

- `lib/runner.py` — `run_all(scenarios=…, provider=…, model=…, binary=…, mode=…, results_path=…)`
- `lib/runtime.py` — `Ctx` dataclass + `make_project` + `wait_for_workflow`
- `lib/server.py` — spawns `nrflo_server` on a fresh `NRFLO_HOME`
- `lib/versions.py` — probes `<binary> --version` for the capability matrix
- `<provider>/__init__.py` — explicit `ALL_SCENARIOS` list for that provider
- `<provider>/test.py` — entry point (`--parallel`, `--model`, `--only`, `--timeout`, `--results`)

Per-provider applicability is recorded in `suite.md` and verified by file presence in each provider folder. Cross-provider gates (`if ctx.provider == …`) are forbidden — if a scenario does not apply to a provider, omit the file.

## Concepts

- **Provider**: `claude`, `codex`, `gemini`, `opencode`, or `python`. CLI providers run under `cli_interactive` (PTY relay); python runs under `script` (execution_mode='script').
- **`Ctx`** (`lib/runtime.py:33`): carries server handle, REST client, provider, model, binary, mode, scenario label.
- **Scenario**: `run(ctx: Ctx) -> Result` where `Result = (name, "PASS"|"FAIL"|"SKIP", details)`. One function per file. Self-contained — no shared fixtures beyond `lib/runtime.py` helpers and `lib/script_helpers.py` for python scenarios.

## Runtime deps

Stdlib only, except `websockets` (required by `s37_ws_event_subscriber`).
Install via `pip install websockets` before running the CLI suites.

## How to run

```bash
make build

# full suite — 5 providers in parallel, scenarios sequential, overwrites /capabilities.md
python3 manual_testing/run_suite.py

# subset of providers
python3 manual_testing/run_suite.py --only=claude,python

# single provider directly (useful for debugging)
python3 manual_testing/claude/test.py --only=s01 --parallel=1
python3 manual_testing/python/test.py --only=P01
```

Each provider subprocess creates `/tmp/nrflo-manual-<provider>-<mode>-XXXX/` with the SQLite DB, per-scenario project roots, and `server.log`. The orchestrator collects results under `/tmp/nrflo-suite-<ts>/`. Directories are kept on exit.

Exit codes: `0` = all PASS/SKIP, `1` = any FAIL, `2` = fatal interruption.

`lib/server.py` gives each server its own `NRFLO_HOME` and `NRFLO_SOCKET` (short `/tmp/...` path; avoids macOS 104-byte AF_UNIX cap and stale-socket conflicts from prior crashes).

## Adding a new scenario

1. Pick the next free id in `suite.md` (`sNN` for CLI, `PNN` for python). Add a one-line description.
2. Create `<provider>/<id>_<short_name>.py` in every provider folder that should run it. Use `claude/s01_findings_save.py` (CLI) or `python/P01_findings_basic.py` (script) as the template. Do not branch on `ctx.provider` inside the file.
3. Append the module to that provider's `__init__.py::ALL_SCENARIOS`.
4. `python3 manual_testing/<provider>/test.py --only=<id> --parallel=1` to debug.
5. Run `python3 manual_testing/run_suite.py` once to regenerate `/capabilities.md`.

## Debugging

- Data dir (`/tmp/nrflo-manual-…`): open `nrflo.data`, query `agent_sessions` and `agent_messages` for the failing workflow instance.
- `server.log` in the same dir: search `ERROR` / `WARN` / `panic`.
- Suite log dir (`/tmp/nrflo-suite-…`): contains `<provider>.json` results and `<provider>.log` stdout.
