---
name: pre-release
description: "Run the full manual_testing suite and report whether everything passed. All seven provider folders (engine, claude, codex, python, api, openai_api, console) launch concurrently, each with its own NRFLO_HOME + NRFLO_SOCKET. Overwrites /capabilities.md with results, CLI versions, nrflo version, and a UTC timestamp. Spawns real CLI processes against real provider credentials; expect ~15–20 minute wall time (gated by the slowest folder, normally engine). Use before cutting a release with /do-release."
---

# /pre-release

End-to-end gate: runs the engine-level scenarios plus every per-provider folder through `manual_testing/run_suite.py` and prints a clean pass/fail report. Invoke as:

```
/pre-release
```

No arguments. If the user passes any, ignore them — the skill is fully self-contained.

## What you're automating

The manual_testing harness exercises the full path "real REST API → real DB → real spawner → real CLI binary → real agent CLI writes back via socket". Per the project layout (see [manual_testing/CLAUDE.md](manual_testing/CLAUDE.md)):

- **`engine`**: the provider-agnostic scenarios (orchestrator/REST/WS/spawner/DB/chains/etc.). Runs under the `claude` binary, mode `cli_interactive`.
- **2 CLI providers**: `claude`, `codex` — each holds only the small set of scenarios whose implementation must diverge per provider (`s05`, `s35`). Both run under `cli_interactive`.
- **`python`**: script-mode scenarios (execution_mode='script') — no LLM, no provider CLI, just python3.
- **`api`**: api-mode scenarios (execution_mode='api') — the in-process Anthropic runner; SKIPs cleanly when no OAuth token is reachable from the macOS Keychain or env (see `manual_testing/lib/credentials.py`).
- **`openai_api`**: api-mode against the OpenAI Responses endpoint; SKIPs when neither `OPENAI_API_KEY` nor `CODEX_API_KEY` is set.
- **`console`**: the console-chat surface (`kind='console_chat'`) — one roundtrip per engine (claude/codex/api) plus an MCP tool-dispatch check; C02/C03 gate on their own binary/credentials.

`run_suite.py` launches every selected provider folder concurrently — each subprocess gets its own isolated `NRFLO_HOME` (`tempfile.mkdtemp`) and `NRFLO_SOCKET` (short `/tmp/...` path) via `lib/server.py`, so the servers don't share DB rows, agent sockets, or HTTP ports. Inside each subprocess scenarios run sequentially (`--parallel=1`). At the end it probes `<binary> --version` for each CLI, reads the repo-root `VERSION` file, and **overwrites** `/capabilities.md` at the repo root with: versions table, scenario-results matrix, per-provider wall times, and a UTC timestamp.

Expected wall time: ~15–20 minutes, gated by the slowest folder (normally `engine` for its sequential scenarios). The other folders finish much faster and idle until engine completes.

**Anthropic API contention**: `engine`, `claude`, `api`, and `console` all hit the Anthropic API. Under one OAuth bearer they can trip rate-limits / induce model-side prompt drift, occasionally flaking long multi-turn scenarios. If a clean baseline is needed (e.g. cutting a release), re-run with `--sequential` so one folder hits Anthropic at a time. The flag preserves the same isolation; it just serialises the launches.

## Step 1 — Preflight (abort on any failure)

Report which check failed and what the user must fix.

```bash
# Must be in repo root.
test -f manual_testing/run_suite.py || { echo "not in nrflo repo root"; exit 1; }

# Provider binaries are optional — run_suite.py marks missing-binary
# providers as SKIPPED in /capabilities.md. Warn so the user knows the
# report is partial. `engine` and `console` reuse the `claude` binary,
# so claude missing also disables those stages.
for bin in claude codex; do
    if ! command -v "$bin" >/dev/null 2>&1; then
        echo "WARN: $bin not on PATH — its column will be SKIPPED"
        [ "$bin" = "claude" ] && echo "WARN: engine + console columns will be SKIPPED too (they run under claude)"
    fi
done

# nrflo_server must be on PATH (single binary — there is no separate nrflo CLI).
command -v nrflo_server >/dev/null 2>&1 || { echo "nrflo_server not on PATH; run: make install"; exit 1; }
```

If any check fails (excluding the missing-provider-binary warnings), stop and tell the user the exact command to fix it.

## Step 1.5 — Model drift check (non-blocking)

While the suite runs (or just before launching it), invoke the `model-audit`
skill (`.claude/skills/model-audit/SKILL.md`): it diffs the
unified `models` registry — per-mode model IDs, contexts, effort lists, and
defaults — against the live codex app-server matrix, the claude CLI effort
enum, and the provider docs. Include its verdict line in the final report.
Drift does **not** fail pre-release; it's release-notes material and a prompt
for a seeds migration.

## Step 2 — Run the suite

```bash
python3 manual_testing/run_suite.py 2>&1 | tee /tmp/pre-release.log
RC=${PIPESTATUS[0]}
```

`run_suite.py` returns 0 if every provider's subprocess exited 0 and no FAIL rows were recorded; 1 otherwise. **Capture the exit code** — don't rely on `tee` masking it. In zsh use `${pipestatus[1]}` instead of `${PIPESTATUS[0]}`.

The run also writes `/capabilities.md` with the new results, CLI versions, nrflo version (from the repo-root `VERSION`), and a UTC timestamp.

## Step 3 — Parse and report

`run_suite.py` itself prints a final block like:

```
=== suite summary ===
  engine       33 pass   0 fail   0 skip   780.00s  OK
  claude        2 pass   0 fail   0 skip    17.00s  OK
  codex         2 pass   0 fail   0 skip    28.00s  OK
  python       20 pass   0 fail   0 skip    40.00s  OK
  api          13 pass   0 fail   0 skip    40.00s  OK
  openai_api    6 pass   0 fail   0 skip    35.00s  OK
  console       4 pass   0 fail   0 skip    90.00s  OK
  --- grid wall: 1140.00s
```

Extract those numbers and re-emit them as the final user-facing report — one row per provider (passed/failed/skipped/wall), then:

```
VERDICT: PASS
/capabilities.md updated.
```

On failure, list each failing scenario per folder:

```
VERDICT: FAIL — 2 folders failed:
  • engine: s17_callback, s19_endless_loop
  • codex:  s35_custom_cli_model
```

The per-folder failing scenarios can be pulled from `/tmp/nrflo-suite-<ts>/<folder>.json` (each file has a `rows` array with `verdict`).

Pad columns so it's column-aligned in monospace.

## Step 4 — Exit status

- If `RC` is 0 **and** every provider's `fail` count is 0, exit 0.
- Otherwise exit 1.

Do not retry, do not auto-restart failing providers. Surface the failure and stop. The point of this gate is to catch regressions before `/do-release` — masking them defeats the purpose.

## When not to use

- **Don't run inside CI.** Local-only; spawns real CLI binaries against the user's real provider credentials.
- **Don't run while another `nrflo_server` is using `~/.nrflo`.** Each provider subprocess boots its own isolated `NRFLO_HOME` under `/tmp`, but a competing dev server on the user's main port can confuse them. Tell them to stop the dev server first.
- **Don't run after a failed `make install`.** The preflight catches `nrflo_server` missing from PATH.

## Notes for future maintainers

- If a new provider is added, drop a `manual_testing/<provider>/` folder with `__init__.py` + `test.py` + scenarios. Then add it to `PROVIDERS` in `run_suite.py`. No skill change needed — the report walks whatever the suite produces.
- Default home for a new scenario is `manual_testing/engine/`. Only put it in a per-provider folder when the implementation must diverge per provider.
- `run_suite.py` already handles missing-binary skip — preserve that behavior; this skill should NEVER fail just because (e.g.) `codex` isn't installed locally.
