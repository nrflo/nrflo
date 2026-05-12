# Manual integration testing harness

Provider/mode-agnostic Python harness that exercises the full path
"real REST API → real DB → real spawner → real CLI binary → real agent
CLI writes back via socket". Lives outside the Go test pyramid because
each run spawns real `claude` / `codex` / `opencode` processes against
real provider credentials — runtime cost is real money.

## Hard rules

- **Manual launch only.** Not wired into `make test`, `make test-ui`,
  `make test-integration`, any other Makefile target, any pre-commit
  hook, or any CI workflow. The only way it runs is when a human types
  `python3 manual_testing/<entry>.py`.
- **No Go test files or vitest test files** live here. No
  `internal/integration/` additions.
- **No Makefile changes** to expose it. If contributors need to discover
  it, link from a feature index — don't surface as a target.

## What it covers

25 scenarios spanning: findings save/get, agent fail/finished/callback,
project findings, message categories, context_left telemetry, skip tags,
layer handoff (`#{FINDINGS:agent:key}` + `#{PRIOR_LAYER_FINDINGS}`),
workflow_final_result, ticket scope + auto-close, parallel agents on a
layer, user_instructions injection, project env var passthrough,
pass_policy=all enforcement, stall detection, layer callback re-spawn,
manual retry-failed, endless_loop bounded, workflow chain run +
chain-next-instructions, chain-next-ticket, next_workflow_on_success
auto-chaining, max_fail_restarts auto-restart loop, agent-session-logs
REST endpoint, findings carryover across fail-restart relaunch (same
`copyFindingsForContinuation` path used by low-context relaunch).

Each scenario is self-contained — prompt(s), workflow config, agent
config, and assertions all live in one file.

## Layout

```
manual_testing/
├── CLAUDE.md               This file
├── lib/
│   ├── api.py              Cookie-based REST client (admin/admin login)
│   ├── db.py               Read-only SQLite helpers (agent_sessions,
│   │                       agent_messages, workflow_instances,
│   │                       project_findings, errors, ticket, chain steps)
│   ├── runner.py           run_all(provider, model, binary, mode, parallel)
│   ├── runtime.py          Ctx dataclass + make_project + wait_for_workflow
│   │                       + next_id + resolve_model helpers
│   └── server.py           Spawns `nrflo_server serve` on fresh NRFLO_HOME
├── scenarios/
│   ├── __init__.py         Explicit ALL_SCENARIOS list (comment out to skip)
│   ├── s01_findings_save.py
│   ├── …                   (25 files, one scenario each)
│   └── s25_findings_carryover.py
├── test_claude.py          --mode={cli,cli-interactive} --parallel --model
├── test_codex.py           same
├── test_opencode.py        same
└── run_all.py              iterates provider × mode grid, prints summary
```

## Concepts

- **Provider**: which CLI binary the agent runs in — `claude`, `codex`,
  or `opencode`. Each has its own `test_<provider>.py` entry.
- **Mode**: how the spawner invokes the CLI. Today: `cli` (batch
  invocation) and `cli-interactive` (PTY relay). `api` and `script`
  groupings are reserved for later (see Tier 3 in `backlog.md`).
- **`Ctx`** (`lib/runtime.py`): carries server handle, REST client,
  provider, model, binary, mode, and a per-scenario log label.
  Scenarios receive a `Ctx` and never construct one.
- **`ALL_SCENARIOS`** (`scenarios/__init__.py`): explicit Python list of
  scenario callables. Reorder or comment out to subset. Each callable
  is `run(ctx: Ctx) -> Result` where `Result = (name, "PASS"|"FAIL"|"SKIP", details)`.

## How to run

```bash
make build                                            # builds be/nrflo_server + be/nrflo
python3 manual_testing/test_claude.py                 # default: mode=cli, parallel=5
python3 manual_testing/test_claude.py --mode=cli-interactive
python3 manual_testing/test_claude.py --parallel=1    # sequential, easier to debug
python3 manual_testing/test_claude.py --model=sonnet  # override the default model

python3 manual_testing/run_all.py                     # provider × mode grid
python3 manual_testing/run_all.py --provider=claude --mode=cli
```

Each run creates `/tmp/nrflo-manual-<provider>-<mode>-XXXX/` containing
the fresh SQLite DB (`nrflo.data`), the per-scenario project root dirs,
and the captured server stdout/stderr (`server.log`). The directory is
**kept on exit** for post-mortem; the harness prints its path at the
end of each run.

### Per-server isolation (NRFLO_HOME + NRFLO_SOCKET)

`lib/server.py` gives every nrflo_server it boots its own:

- **`NRFLO_HOME`** — fresh tempdir as above; this is also where the SQLite
  DB lives.
- **`NRFLO_SOCKET`** — short `/tmp/nrflo-manual-<cli_label>-<pid>.sock`
  path, set explicitly on the server's env. Two reasons we don't rely on
  the `$NRFLO_HOME/agent.sock` default:
  1. macOS caps AF_UNIX paths at 104 bytes — `$NRFLO_HOME` under
     `/var/folders/...` can push the joined path close to the limit.
  2. Explicit per-server socket paths match the Go integration harness
     (`be/internal/integration/testenv_test.go`), so a stale socket from
     a crashed prior run can't make a fresh server fail its eager bind.

Within a single (provider × mode) subprocess, all scenarios share one
server (and therefore one socket — the socket routes by session_id, so
parallel scenarios don't collide). Across subprocesses started by
`run_all.py`, each has its own PID → its own socket. `RunningServer.stop`
always removes the socket file even when keeping `NRFLO_HOME` for
post-mortem.

Exit codes: `0` = all PASS/SKIP, `1` = at least one FAIL, `2` = fatal
interruption (KeyboardInterrupt).

## Per-scenario file structure

Each `scenarios/sNN_<name>.py` follows the same shape:

```python
"""SNN — Human title.

Tests:
  - Concrete bullet of what's being exercised.
Expected PASS result:
  - Concrete bullet of what the DB / REST should look like.
"""

from __future__ import annotations

from lib import db as db_mod
from lib.runtime import (
    Ctx, Result, make_project, next_id, resolve_model, wait_for_workflow,
)

# Per-provider model overrides; empty = use the runner default.
MODELS_BY_PROVIDER: dict[str, str] = {}

PROMPT = """\
You are an integration-test agent. Do EXACTLY what is listed below and
nothing else. Use the Bash tool to run the listed commands in order,
then stop immediately.

1. Run: `nrflo …`
2. Run: `nrflo agent finished`
"""


def run(ctx: Ctx) -> Result:
    pid, _root = make_project(ctx)
    wid = next_id(ctx, "wf")
    ctx.client.create_workflow(pid, wid, scope_type="project")
    ctx.client.create_agent_def(
        pid, wid, "main",
        model=resolve_model(ctx, MODELS_BY_PROVIDER),
        layer=0, timeout=5, prompt=PROMPT,
    )
    wfi = ctx.client.run_project_workflow(
        pid, wid, instructions="…",
    )["instance_id"]
    wait_for_workflow(ctx, pid, instance_id=wfi)

    # assertions … return ("SNN name", "PASS"|"FAIL", details)
```

Conventions:

- **Self-contained** — prompts, workflow scope/groups/policy, agent_def
  config (model, timeout, layer, stall, max_fail_restarts) all live in
  the file. Slight duplication between files is acceptable; the goal is
  for one file = full test setup at a glance.
- **Structured docstring** — `Tests:` and `Expected PASS result:` at the
  top so you can `head` a file and know what it does.
- **`MODELS_BY_PROVIDER`** — drop in `{"claude": "sonnet", "codex":
  "codex_gpt_high"}` when haiku/default model can't follow the prompt
  reliably on a particular provider. Empty = default.
- **Assertions in the scenario body** — `first_session` + DB helpers +
  early-return tuples. Don't push assertions into shared helpers; reading
  one file should answer "what did this test prove?".

## Adding a new scenario

1. Create `scenarios/sNN_<short_name>.py` from the template above.
2. Append two lines to `scenarios/__init__.py`:
   ```python
   from . import sNN_short_name
   ALL_SCENARIOS.append(sNN_short_name.run)
   ```
3. `python3 manual_testing/test_claude.py --parallel=1` to debug. The
   `--parallel=1` keeps log lines un-interleaved.

## Modes & execution_mode propagation

`Ctx.mode` is `"cli"` or `"cli-interactive"`. Selection is enforced at
the **agent_definition** level, not the project level:

- `cli` — agent_definitions are created without an `execution_mode`
  field, so the backend defaults to `cli` (batch invocation).
- `cli-interactive` — the runner sets
  `client.default_execution_mode = "cli_interactive"` once after login
  (in `lib/runner.py`). Every `create_agent_def` call in `lib/api.py`
  reads that attr and adds `"execution_mode": "cli_interactive"` to its
  POST body, so each spawned agent routes through the PTY interactive
  backend (`be/internal/spawner/backend_interactive.go`).

Why per-agent and not per-project: commits `a3305e3`, `53c9e84`, and
`6849af7` removed the project-level `interactive_cli_mode` toggle and
made `cli_interactive` a first-class `execution_mode` alongside `cli`,
`api`, and `script`. `make_project` no longer touches project config.

Scenarios don't branch on mode. The exception is `s16_stall_detection`,
which gracefully `SKIP`s under `cli-interactive` because PTY relay
produces a steady byte stream that defeats the running-stall timer
(see [be/internal/spawner/CLAUDE.md](../be/internal/spawner/CLAUDE.md)
→ stall detection).

## Verbose output + timings

Runner prints one line per scenario transition:

```
[HH:MM:SS] [claude/cli] [3/24] >>> s03_project_findings
        [s03_project_findings] polling workflow 7ee31e13… (project=p-claude-11, ticket=-)
        [s03_project_findings] status=active (after 0.0s)
        [s03_project_findings] status=project_completed (after 8.0s)
[HH:MM:SS] [claude/cli] [3/24] ✓ PASS s03_project_findings ( 8.14s) — keys=['team']
```

Final per-combo table reports:
- per-scenario verdict, wall time, details
- aggregate: scenarios count, fails count, in-scenario sum, suite wall,
  speedup (`in-scenario / wall`) — a proxy for how well parallelism is
  saturating the LLM provider.

## Runtime knobs

| Constant | File | Purpose |
|---|---|---|
| `RUN_TIMEOUT_S` | `lib/runtime.py` | Hard deadline per `wait_for_workflow` call (180s). |
| `POLL_INTERVAL_S` | `lib/runtime.py` | Workflow-state poll interval (0.5s; cheap REST). |
| `MARK` | `lib/runner.py` | PASS/FAIL/SKIP glyphs. |
| `MODELS_BY_PROVIDER` | each scenario | Per-provider model override; empty = use entry script default. |

## Known broken provider × mode combos

Tracked in `backlog.md` ("Backend issues surfaced by the manual-testing
harness"):

- `codex/cli-interactive` — codex TUI emits zero bytes to the spawner's
  PTY reader; every scenario times out. Investigate
  `be/internal/spawner/cli_adapter_codex.go` interactive path.
- `opencode/cli-interactive` — agents exit within 1-2s with
  `result_reason='exit_code'`; opencode 1.14.48 interactive invocation
  args drift suspected. Investigate
  `be/internal/spawner/cli_adapter_opencode.go`.
- `claude/cli-interactive` — under suite-level `--parallel=5` load,
  one PTY session (s10's first attempt observed it, but the bug is
  load-sensitive, not s10-specific) reaches `deliverPrompt: submitted`
  then produces zero further events until `server_shutdown` fails it.
  Re-running the offending scenario in isolation (`--parallel=1
  --only=…`) passes cleanly, so the trigger is **total concurrent PTY
  count on the server**, not within-workflow concurrency. First
  surfaced 2026-05-12 after `lib/runner.py` was fixed to set
  `default_execution_mode = "cli_interactive"` (per-agent toggle, post
  mig-101 / 6849af7 / a3305e3 / 53c9e84) — cli-interactive scenarios
  had been silently running through plain `cli` since those commits
  landed. See `backlog.md` section "Backend issues surfaced by the
  manual-testing harness" for full triage notes.

None of these combos is gated in the harness — run them yourself to
see the failure. Fix the backend or skip those provider+mode combos
until the upstream/adapter issue is resolved; either resolution closes
the backlog entries.

## Common pitfalls

- **Forgot to `make build`** — `lib/server.py` exits with a clear error
  pointing at the missing `be/nrflo_server` / `be/nrflo` binaries.
- **Provider CLI missing on PATH** — the runner SKIPs and exits 0 for
  that combo (no failure).
- **REST 400 with `"… is required"`** — the project_workflow handlers
  validate body fields in a strict order (workflow → instance_id →
  session_id). Pass them all. See `s18_retry_failed.py` for the
  retry-failed surface (needs all three).
- **`workflow_instances.skip_tags` won't accept your tag** — the
  workflow.skip socket handler validates the tag against
  `workflow.groups`. Set `groups=["your-tag"]` at workflow creation
  (see `s06_skip_tag.py`).
- **Templating substitution doesn't work** — the spawner uses
  `#{FINDINGS:agent:key}` / `#{PROJECT_FINDINGS:key}` / `#{PRIOR_LAYER_FINDINGS}`,
  not `${findings.key}`. See `s07_layer_handoff.py` /
  `s12_project_findings_xwf.py` / `s15_prior_layer_findings.py`.
- **`endless_loop=true` doesn't show up on the same wfi** — each
  iteration creates a NEW `workflow_instances` row. Use
  `db.workflow_instances_for_workflow(home, project, workflow_id)`,
  not `agent_sessions_for_instance`. See `s19_endless_loop.py`.
- **Codex/opencode skip your callback** — model instruction-following
  varies. Drop a `MODELS_BY_PROVIDER` override on that scenario.

## Where to look first when something breaks

1. The aggregate table — find the FAILing scenario name.
2. The kept data dir (`/tmp/nrflo-manual-…`) — open `nrflo.data` in any
   SQLite browser and `SELECT * FROM agent_sessions WHERE
   workflow_instance_id = '<wfi from log>'`. Check `result`,
   `result_reason`, `findings`.
3. `server.log` in the same dir — search for `ERROR` / `WARN` /
   `panic`. Most agent failures log a one-liner.
4. `agent_messages` — `SELECT seq, category, substr(content, 1, 200)
   FROM agent_messages WHERE session_id = '<id>'`. If the agent never
   ran your command, you'll see it in the tool/text rows.
