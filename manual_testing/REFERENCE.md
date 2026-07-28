# Manual Testing Reference

Deep mechanics for this harness. The auto-loaded map lives in [CLAUDE.md](CLAUDE.md); read the relevant section here before adding scenarios or debugging runs.

Contents: [Layout](#layout) · [Concepts](#concepts) · [Adding a new scenario](#adding-a-new-scenario) · [Debugging](#debugging)

## Layout

```
manual_testing/
├── suite.md                 # canonical scenario catalogue (numbers + descriptions)
├── run_suite.py             # orchestrator: all provider folders run concurrently (per-folder NRFLO_HOME + NRFLO_SOCKET)
├── lib/                     # shared infra: api, db, runner, runtime, server, ws_client, http_mock, script_helpers, versions, credentials
├── engine/                  # provider-agnostic scenarios, run under the claude binary
├── claude/                  # claude-specific scenarios only (s05, s35)
├── codex/                   # codex-specific scenarios only (s05, s35)
├── python/                  # execution_mode='script' scenarios (no CLI, no LLM)
├── api/                     # execution_mode='api' scenarios (in-process Anthropic runner)
├── openai_api/              # execution_mode='api' scenarios (OpenAI Responses endpoint)
└── console/                 # console-chat surface (kind='console_chat'), one scenario per engine
```

- `lib/runner.py` — `run_all(scenarios=…, provider=…, model=…, binary=…, mode=…, results_path=…)`
- `lib/runtime.py` — `Ctx` dataclass + `make_project` + `wait_for_workflow`
- `lib/server.py` — spawns `nrflo_server` on a fresh `NRFLO_HOME`
- `lib/versions.py` — probes `<binary> --version` for the capability matrix
- `lib/console.py` — helpers for the console-chat REST surface (create/list/detail/messages/close, yolo toggle)
- `<folder>/__init__.py` — explicit `ALL_SCENARIOS` list for that folder
- `<folder>/test.py` — entry point (`--parallel`, `--only`, `--timeout`, `--results`; `python/test.py` has no `--model` flag, its model is fixed to `haiku-4-5`)

## Concepts

- **Provider**: `engine`, `claude`, `codex`, `python`, `api`, `openai_api`, or `console`. `console` boots under the `claude` binary and drives `kind='console_chat'` sessions over REST — its scenarios are engine-scoped (C02 gates on `codex`, C03 on an OAuth token) rather than provider-scoped. `engine` and the CLI providers run under `cli_interactive` (PTY relay) — `engine` uses the `claude` binary. `python` runs under `script` (execution_mode='script'). `api` runs the in-process Anthropic runner (`execution_mode='api'`); SKIPs when `lib/credentials.probe_oauth_token()` cannot resolve an OAuth bearer token. `openai_api` runs the in-process OpenAI runner (`execution_mode='api'`) on an OpenAI-provider row from the unified models registry; SKIPs when `lib/credentials.probe_openai_key()` finds neither `OPENAI_API_KEY` nor `CODEX_API_KEY`.
- **`Ctx`** (`lib/runtime.py:33`): carries server handle, REST client, provider, model, binary, mode, scenario label.
- **Scenario**: `run(ctx: Ctx) -> Result` where `Result = (name, "PASS"|"FAIL"|"SKIP", details)`. One function per file. Self-contained — no shared fixtures beyond `lib/runtime.py` helpers and `lib/script_helpers.py` for python scenarios.

## Adding a new scenario

1. Pick the next free id in `suite.md` (`sNN` for CLI, `PNN` for python, `ANN` for api-mode, `ONN` for openai api-mode, `CNN` for console-chat). Add a one-line description.
2. Default home is `engine/`. Create `engine/<id>_<short_name>.py` using `engine/s02_agent_fail.py` (CLI) or `python/P01_findings_basic.py` (script) as the template. Do not branch on `ctx.provider` inside the file. Only put a scenario in a per-provider folder when the implementation must diverge per provider — in that case add the file to every applicable provider folder.
3. Append the module to that folder's `__init__.py::ALL_SCENARIOS`.
4. `python3 manual_testing/<folder>/test.py --only=<id> --parallel=1` to debug.
5. Run `python3 manual_testing/run_suite.py` once to regenerate `/capabilities.md`.

## Debugging

- Data dir (`/tmp/nrflo-manual-…`): open `nrflo.data`, query `agent_sessions` and `agent_messages` for the failing workflow instance.
- `server.log` in the same dir: search `ERROR` / `WARN` / `panic`.
- Suite log dir (`/tmp/nrflo-suite-…`): contains `<provider>.json` results and `<provider>.log` stdout.
