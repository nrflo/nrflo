# Manual integration testing harness

Provider-agnostic Python harness that exercises the full path "real REST API → real DB → real spawner → real CLI binary → real agent CLI writes back via socket". Lives outside the Go test pyramid — each run spawns real `claude` / `codex` / `opencode` processes against real provider credentials.

## Hard rules

- **Manual launch only.** Not wired into any Makefile target, CI workflow, or pre-commit hook.
- **No Go test files or vitest test files** live here.
- **No Makefile changes** to expose it.

## What it covers

CLI-provider scenarios ({claude,codex,opencode} × cli_interactive): 37 scenarios across findings, callbacks, pass policies, stall detection, endless loops, chains, context save/resume (both resume and agent-saver branches), manual restart, ticket concurrency 409, take-control/exit-interactive, plan_mode, multi-instance-same-ticket, custom cli_models, WS subscriber, and notification webhook. One scenario (s28) is a SKIP stub for the codex resume-fallback path — see `backlog.md` §7. See `scenarios/__init__.py` for the full `ALL_SCENARIOS` list.

Script-backend scenarios (`execution_mode='script'`, no provider CLI, no LLM): 18 scenarios exercising every method of the embedded `nrflo_sdk` (findings, project_findings, agent control incl. `chain_next_ticket`, context/user_instructions/callback_info/previous_data, skip incl. multi-tag accumulation, log with each category) plus project env vars, exception → fail, stderr capture, chain handoff via SDK. See `scenarios_script/__init__.py` for the full `ALL_SCRIPT_SCENARIOS` list.

## Layout

- `lib/api.py` — Cookie-based REST client (admin/admin login)
- `lib/db.py` — Read-only SQLite helpers
- `lib/runner.py` — `run_all(provider, model, binary, parallel)` for CLI providers; `run_scripts(parallel)` for script-mode
- `lib/runtime.py` — `Ctx` dataclass + `make_project` + `wait_for_workflow` helpers
- `lib/script_helpers.py` — `make_script` / `make_script_agent` helpers + `SDK_BOOTSTRAP` prelude
- `lib/server.py` — Spawns nrflo_server on fresh NRFLO_HOME
- `lib/ws_client.py` — Sync WebSocket subscriber (cookie-authed) — used by s37
- `lib/http_mock.py` — `WebhookCapture` in-process httpserver — used by s38
- `scenarios/__init__.py` — `ALL_SCENARIOS` (CLI providers; comment out to skip)
- `scenarios_script/__init__.py` — `ALL_SCRIPT_SCENARIOS` (script backend)
- `scenarios/s01_findings_save.py`, `s25_findings_carryover.py` — example CLI scenarios
- `scenarios_script/ps01_findings_basic.py`, `ps12_log_categories.py` — example script scenarios
- `test_claude.py`, `test_codex.py`, `test_opencode.py` — CLI-provider entry points (`--parallel --model`)
- `test_script.py` — script-mode entry point (`--parallel`); no `--model`
- `run_all.py` — iterates over providers; `script` is a synthetic provider with one mode (`native`)

## Concepts

- **Provider**: which CLI binary the agent runs in — `claude`, `codex`, or `opencode`.
- **`Ctx`** (`lib/runtime.py`): carries server handle, REST client, provider, model, binary, and a per-scenario log label. All CLI providers run under `cli_interactive` (PTY relay).
- **`ALL_SCENARIOS`** (`scenarios/__init__.py`): explicit list of callables; each is `run(ctx: Ctx) -> Result` where `Result = (name, "PASS"|"FAIL"|"SKIP", details)`.

## Runtime deps

Stdlib only, except `websockets` (required by `s37_ws_event_subscriber`).
Install via `pip install websockets` before running the CLI suites.

## How to run

```bash
make build
python3 manual_testing/test_claude.py                  # parallel=5
python3 manual_testing/test_claude.py --parallel=1     # sequential, easier to debug
python3 manual_testing/test_claude.py --model=sonnet
python3 manual_testing/test_script.py                  # script backend, no LLM
python3 manual_testing/test_script.py --only=ps01,ps12 # subset
python3 manual_testing/run_all.py                      # all providers (incl. script)
python3 manual_testing/run_all.py --provider=script    # script only
python3 manual_testing/run_all.py --provider=claude
```

Each run creates `/tmp/nrflo-manual-<provider>-cli_interactive-XXXX/` with the SQLite DB, per-scenario project roots, and `server.log`. Directory is kept on exit. Exit codes: `0` = all PASS/SKIP, `1` = any FAIL, `2` = fatal interruption.

`lib/server.py` gives each server its own `NRFLO_HOME` and `NRFLO_SOCKET` (short `/tmp/...` path; avoids macOS 104-byte AF_UNIX cap and stale-socket conflicts from prior crashes). All scenarios within one subprocess share one server; across subprocesses started by `run_all.py`, each has its own PID and socket.

## Adding a new scenario

CLI-provider:
1. Create `scenarios/sNN_<short_name>.py` from the template at `scenarios/s01_findings_save.py`.
2. Append to `scenarios/__init__.py`:
   ```python
   from . import sNN_short_name
   ALL_SCENARIOS.append(sNN_short_name.run)
   ```
3. `python3 manual_testing/test_claude.py --parallel=1` to debug.

Script-backend:
1. Create `scenarios_script/psNN_<short_name>.py` from the template at `scenarios_script/ps01_findings_basic.py`. Use `lib.script_helpers.make_script_agent(...)` to wire the agent; the SDK bootstrap (`import nrflo_sdk; c = nrflo_sdk.client()`) is prepended automatically.
2. Append to `scenarios_script/__init__.py`.
3. `python3 manual_testing/test_script.py --parallel=1 --only=psNN` to debug.

## Debugging

- Data dir (`/tmp/nrflo-manual-…`): open `nrflo.data`, query `agent_sessions` and `agent_messages` for the failing workflow instance.
- `server.log` in the same dir: search `ERROR` / `WARN` / `panic`.
- Known provider-specific issues are tracked in `backlog.md`.
