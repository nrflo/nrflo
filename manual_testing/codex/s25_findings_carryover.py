"""S25 — Findings carry over to relaunched session on fail-restart.

Tests:
  - Agent #1 writes a finding (`carry_key=carry_val`), then calls
    `nrflo agent fail`. With `max_fail_restarts=1` the spawner auto-
    respawns via `relaunchForContinuation`, which calls
    `copyFindingsForContinuation` to merge old findings into the new
    session row non-destructively (new keys win on conflict).
  - Agent #2 just calls `nrflo agent finished` without writing any
    finding of its own, so the merged value is observable on the new
    session row.

Note:
  - The same `copyFindingsForContinuation` function is invoked from the
    low-context relaunch path (`context_save.go` → resume-based or
    system-agent save → `relaunchForContinuation`). Triggering low
    context deterministically in a manual test would require either
    burning real context tokens or driving `restart_threshold` very high
    and spawning a `context-saver` system agent; the underlying carryover
    is unit-tested in `findings_carryover_test.go`, so this scenario
    covers only the fail-restart trigger.

Expected PASS result:
  - 2 agent_sessions rows for the wfi.
  - sessions[0].result == 'continue' and result_reason == 'fail_restart'
    (fail-restart override) with findings.carry_key == 'carry_val'.
  - sessions[1].findings.carry_key == 'carry_val' (carried over).
"""

from __future__ import annotations

from lib import db as db_mod
from lib.runtime import (
    Ctx, Result, make_project, next_id, resolve_model, wait_for_workflow,
)


# Per-provider model overrides; empty = use the runner default (e.g. haiku).
MODELS_BY_PROVIDER: dict[str, str] = {}


PROMPT = """\
You are an integration-test agent. Behavior depends on whether the
finding `carry_key` is already set on YOUR session.

First, run: `nrflo findings get carry_key`

- If the output is empty or shows "no finding", this is the FIRST
  attempt. Run, in order:
    1. `nrflo findings add carry_key carry_val`
    2. `nrflo agent fail --reason "trigger fail-restart"`
  Then stop.

- If the output is `carry_val`, this is the RELAUNCHED attempt. Run:
    1. `nrflo agent finished`
  Then stop. Do NOT call `nrflo findings add` — we want to observe the
  carried-over value untouched.
"""


def run(ctx: Ctx) -> Result:
    pid, _root = make_project(ctx)
    wid = next_id(ctx, "wf")
    ctx.client.create_workflow(pid, wid, scope_type="project")
    ctx.client.create_agent_def(
        pid, wid, "main",
        model=resolve_model(ctx, MODELS_BY_PROVIDER),
        layer=0, timeout=5, prompt=PROMPT,
        max_fail_restarts=1,
    )
    wfi = ctx.client.run_project_workflow(
        pid, wid, instructions="fail-restart carryover",
    )["instance_id"]
    wait_for_workflow(ctx, pid, instance_id=wfi)

    sessions = db_mod.agent_sessions_for_instance(ctx.server.home, wfi)
    if len(sessions) != 2:
        return ("S25 findings carryover", "FAIL",
                f"session count = {len(sessions)}, want 2 (initial + 1 fail-restart)")

    first, second = sessions[0], sessions[1]
    if first.get("result") != "continue" or first.get("result_reason") != "fail_restart":
        return ("S25 findings carryover", "FAIL",
                f"session[0] result/reason = {first.get('result')}/{first.get('result_reason')}, "
                "want continue/fail_restart")

    first_val = (first.get("findings") or {}).get("carry_key")
    if first_val != "carry_val":
        return ("S25 findings carryover", "FAIL",
                f"session[0].findings.carry_key = {first_val!r}, want 'carry_val'")

    second_val = (second.get("findings") or {}).get("carry_key")
    if second_val != "carry_val":
        return ("S25 findings carryover", "FAIL",
                f"session[1].findings.carry_key = {second_val!r}, "
                "want 'carry_val' (carried over from session[0])")

    return ("S25 findings carryover", "PASS",
            f"carry_key persisted across fail-restart (session[1]={second['id']})")
