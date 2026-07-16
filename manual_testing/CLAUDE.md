# Manual integration testing harness

Per-provider Python harness that exercises the full path "real REST API → real DB → real spawner → real CLI binary → real agent CLI writes back via socket". Lives outside the Go test pyramid — each run spawns real CLI processes against real provider credentials. Deep mechanics (layout, concepts, scenario authoring, debugging): [REFERENCE.md](REFERENCE.md) — read it before adding a scenario or debugging a failed run.

## Hard rules

- **Manual launch only.** Not wired into any Makefile target, CI workflow, or pre-commit hook.
- **No Go test files or vitest test files** live here.
- **No Makefile changes** to expose it.

## Layout

One folder per provider (`engine`, `claude`, `codex`, `python`, `api`, `openai_api`) plus the engine-scoped `console` folder (console-chat surface) and shared `lib/`; `suite.md` is the canonical scenario catalogue and `run_suite.py` runs all folders concurrently with isolated `NRFLO_HOME`/`NRFLO_SOCKET`. Full tree + per-file roles: [REFERENCE.md](REFERENCE.md#layout) — read before adding files or folders.

Folder applicability is recorded in `suite.md` and verified by file presence in each folder. Cross-provider gates (`if ctx.provider == …`) are forbidden inside scenarios — divergent behaviour belongs in a per-provider folder.

## Concepts

Providers map to execution modes (`cli_interactive` / `script` / `api`) and SKIP when credentials are missing; a scenario is a self-contained `run(ctx: Ctx) -> Result` function, one per file. Details: [REFERENCE.md](REFERENCE.md#concepts) — read before writing a scenario.

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
python3 manual_testing/openai_api/test.py --only=O01 --parallel=1
python3 manual_testing/console/test.py --only=C01 --parallel=1
```

Each provider subprocess creates `/tmp/nrflo-manual-<provider>-<mode>-XXXX/` with the SQLite DB, per-scenario project roots, and `server.log`. The orchestrator collects results under `/tmp/nrflo-suite-<ts>/`. Directories are kept on exit.

Exit codes: `0` = all PASS/SKIP, `1` = any FAIL, `2` = fatal interruption.

`lib/server.py` gives each server its own `NRFLO_HOME` and `NRFLO_SOCKET` (short `/tmp/...` path; avoids macOS 104-byte AF_UNIX cap and stale-socket conflicts from prior crashes).

## Adding a new scenario / Debugging

Step-by-step scenario authoring: [REFERENCE.md](REFERENCE.md#adding-a-new-scenario). Failed-run triage (data dirs, `server.log`, suite logs): [REFERENCE.md](REFERENCE.md#debugging).
