"""S19 — Endless-loop bounded by stop flag.

Tests:
  - With endless_loop=true on the run, each successful workflow_instance
    spawns the next one for the same workflow_id (a NEW wfi row each
    iteration — not new sessions on the same row).
  - After observing ≥ 2 iterations, set stop_endless_loop_after_iteration
    on the active iteration; the orchestrator must complete it and stop
    spawning more.

Expected PASS result:
  - ≥ 2 workflow_instances rows for the workflow_id with endless_loop=1.
  - Active iteration ends with status ∈ {completed, project_completed}.
"""

from __future__ import annotations

import time

from lib import db as db_mod
from lib.runtime import (
    Ctx, PASS_STATUSES, Result,
    make_project, next_id, resolve_model, wait_for_workflow,
)


# Per-provider model overrides; empty = use the runner default (e.g. haiku).
MODELS_BY_PROVIDER: dict[str, str] = {}


PROMPT = """\
You are an integration-test agent. Do EXACTLY what is listed below and
nothing else. Use the Bash tool to run the listed command, then stop.

1. Run: `nrflo agent finished`
"""


def run(ctx: Ctx) -> Result:
    pid, _root = make_project(ctx)
    wid = next_id(ctx, "wf")
    ctx.client.create_workflow(pid, wid, scope_type="project")
    ctx.client.create_agent_def(
        pid, wid, "main",
        model=resolve_model(ctx, MODELS_BY_PROVIDER), layer=0, timeout=5, prompt=PROMPT,
    )
    ctx.client.run_project_workflow(pid, wid, endless_loop=True)

    deadline = time.monotonic() + 90.0
    while time.monotonic() < deadline:
        instances = db_mod.workflow_instances_for_workflow(
            ctx.server.home, pid, wid)
        if len(instances) >= 2:
            break
        time.sleep(2)
    instances = db_mod.workflow_instances_for_workflow(
        ctx.server.home, pid, wid)
    if len(instances) < 2:
        return ("S19 endless loop", "FAIL",
                f"only {len(instances)} iteration(s) spawned in 90s")

    active = next((i for i in instances if i.get("status") == "active"), None)
    if active:
        ctx.client.stop_endless_loop(pid, instance_id=active["id"], stop=True)
        wait_for_workflow(ctx, pid, instance_id=active["id"])

    final = db_mod.workflow_instance(
        ctx.server.home, (active or instances[-1])["id"])
    if (final or {}).get("status") not in PASS_STATUSES:
        return ("S19 endless loop", "FAIL",
                f"final status = {final.get('status') if final else None}")
    return ("S19 endless loop", "PASS",
            f"{len(instances)} iterations spawned, stop flag honored")
