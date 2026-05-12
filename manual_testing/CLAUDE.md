# Manual integration testing harness

Provider/mode-agnostic Python harness that exercises the full path "real REST API → real DB → real spawner → real CLI binary → real agent CLI writes back via socket". Lives outside the Go test pyramid — each run spawns real `claude` / `codex` / `opencode` processes against real provider credentials.

## Hard rules

- **Manual launch only.** Not wired into any Makefile target, CI workflow, or pre-commit hook.
- **No Go test files or vitest test files** live here.
- **No Makefile changes** to expose it.

## What it covers

25 scenarios across findings, callbacks, pass policies, stall detection, endless loops, chains, and more. See `scenarios/__init__.py` for the full `ALL_SCENARIOS` list.

## Layout

- `lib/api.py` — Cookie-based REST client (admin/admin login)
- `lib/db.py` — Read-only SQLite helpers
- `lib/runner.py` — run_all(provider, model, binary, mode, parallel)
- `lib/runtime.py` — Ctx dataclass + make_project + wait_for_workflow helpers
- `lib/server.py` — Spawns nrflo_server on fresh NRFLO_HOME
- `scenarios/__init__.py` — ALL_SCENARIOS list (comment out to skip)
- `scenarios/s01_findings_save.py`, `s25_findings_carryover.py` — example scenarios
- `test_claude.py` — --mode={cli,cli-interactive} --parallel --model
- `test_codex.py`, `test_opencode.py` — provider entry points
- `run_all.py` — iterates provider × mode grid

## Concepts

- **Provider**: which CLI binary the agent runs in — `claude`, `codex`, or `opencode`.
- **Mode**: `cli` (batch invocation) or `cli-interactive` (PTY relay).
- **`Ctx`** (`lib/runtime.py`): carries server handle, REST client, provider, model, binary, mode, and a per-scenario log label.
- **`ALL_SCENARIOS`** (`scenarios/__init__.py`): explicit list of callables; each is `run(ctx: Ctx) -> Result` where `Result = (name, "PASS"|"FAIL"|"SKIP", details)`.

## How to run

```bash
make build
python3 manual_testing/test_claude.py                  # mode=cli, parallel=5
python3 manual_testing/test_claude.py --mode=cli-interactive
python3 manual_testing/test_claude.py --parallel=1     # sequential, easier to debug
python3 manual_testing/test_claude.py --model=sonnet
python3 manual_testing/run_all.py                      # provider × mode grid
python3 manual_testing/run_all.py --provider=claude --mode=cli
```

Each run creates `/tmp/nrflo-manual-<provider>-<mode>-XXXX/` with the SQLite DB, per-scenario project roots, and `server.log`. Directory is kept on exit. Exit codes: `0` = all PASS/SKIP, `1` = any FAIL, `2` = fatal interruption.

`lib/server.py` gives each server its own `NRFLO_HOME` and `NRFLO_SOCKET` (short `/tmp/...` path; avoids macOS 104-byte AF_UNIX cap and stale-socket conflicts from prior crashes). All scenarios within one subprocess share one server; across subprocesses started by `run_all.py`, each has its own PID and socket.

## Adding a new scenario

1. Create `scenarios/sNN_<short_name>.py` from the template at `scenarios/s01_findings_save.py`.
2. Append to `scenarios/__init__.py`:
   ```python
   from . import sNN_short_name
   ALL_SCENARIOS.append(sNN_short_name.run)
   ```
3. `python3 manual_testing/test_claude.py --parallel=1` to debug.

## Debugging

- Data dir (`/tmp/nrflo-manual-…`): open `nrflo.data`, query `agent_sessions` and `agent_messages` for the failing workflow instance.
- `server.log` in the same dir: search `ERROR` / `WARN` / `panic`.
- Known provider × mode issues are tracked in `backlog.md`.
