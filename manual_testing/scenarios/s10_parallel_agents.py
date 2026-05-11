"""S10 — Parallel agents on the same layer.

Tests:
  - Two agents at layer 0 run concurrently and each writes its own finding.
  - Per-agent findings rows survive the concurrent writes.

Expected PASS result:
  - Both agent sessions exist with result='pass'.
  - Session 'a'.findings.from_a == 'alpha'
  - Session 'b'.findings.from_b == 'beta'
"""

from __future__ import annotations

from lib import db as db_mod
from lib.runtime import Ctx, Result, make_project, next_id, resolve_model, wait_for_workflow


# Per-provider model overrides; empty = use the runner default (e.g. haiku).
MODELS_BY_PROVIDER: dict[str, str] = {}


PROMPT_A = """\
You are an integration-test agent. Do EXACTLY what is listed below and
nothing else. Use the Bash tool to run the listed commands in order,
then stop immediately.

1. Run: `nrflo findings add from_a alpha`
2. Run: `nrflo agent finished`
"""

PROMPT_B = """\
You are an integration-test agent. Do EXACTLY what is listed below and
nothing else. Use the Bash tool to run the listed commands in order,
then stop immediately.

1. Run: `nrflo findings add from_b beta`
2. Run: `nrflo agent finished`
"""


def run(ctx: Ctx) -> Result:
    pid, _root = make_project(ctx)
    wid = next_id(ctx, "wf")
    ctx.client.create_workflow(pid, wid, scope_type="project")
    ctx.client.create_agent_def(
        pid, wid, "a", model=resolve_model(ctx, MODELS_BY_PROVIDER), layer=0, timeout=5, prompt=PROMPT_A)
    ctx.client.create_agent_def(
        pid, wid, "b", model=resolve_model(ctx, MODELS_BY_PROVIDER), layer=0, timeout=5, prompt=PROMPT_B)
    wfi = ctx.client.run_project_workflow(
        pid, wid, instructions="parallel",
    )["instance_id"]
    wait_for_workflow(ctx, pid, instance_id=wfi)

    by_type = {s["agent_type"]: s for s in
               db_mod.agent_sessions_for_instance(ctx.server.home, wfi)}
    if "a" not in by_type or "b" not in by_type:
        return ("S10 parallel agents", "FAIL",
                f"missing sessions: {list(by_type)}")
    a_f = (by_type["a"].get("findings") or {}).get("from_a")
    b_f = (by_type["b"].get("findings") or {}).get("from_b")
    if a_f != "alpha" or b_f != "beta":
        return ("S10 parallel agents", "FAIL",
                f"findings a={a_f!r} b={b_f!r}")
    return ("S10 parallel agents", "PASS", "concurrent findings survived")
