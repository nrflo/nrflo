"""S22 — max_fail_restarts auto-restart on agent failure.

Tests:
  - Agent definition with max_fail_restarts=2 must auto-respawn after
    the agent calls `nrflo agent fail`, up to 2 additional times.
  - Each spawn is a fresh agent_sessions row on the same wfi, with
    restart_count incrementing.

Expected PASS result:
  - workflow_instances ends with status='failed' (all restarts also fail).
  - agent_sessions count for the wfi == 3 (initial + 2 restarts).
  - The 2 restart rows have restart_count > 0.
"""

from __future__ import annotations

from lib import db as db_mod
from lib.runtime import (
    Ctx, Result, make_project, next_id, resolve_model, wait_for_workflow,
)


# Per-provider model overrides; empty = use the runner default (e.g. haiku).
MODELS_BY_PROVIDER: dict[str, str] = {}


PROMPT = """\
You are an integration-test agent. Do EXACTLY what is listed below and
nothing else. Run the command via the Bash tool, then stop.

1. Run: `nrflo agent fail --reason "restart-me"`
"""


def run(ctx: Ctx) -> Result:
    pid, _root = make_project(ctx)
    wid = next_id(ctx, "wf")
    ctx.client.create_workflow(pid, wid, scope_type="project")
    ctx.client.create_agent_def(
        pid, wid, "main",
        model=resolve_model(ctx, MODELS_BY_PROVIDER),
        layer=0, timeout=5, prompt=PROMPT,
        max_fail_restarts=2,
    )
    wfi = ctx.client.run_project_workflow(
        pid, wid, instructions="auto restart",
    )["instance_id"]
    wait_for_workflow(ctx, pid, instance_id=wfi)

    inst = db_mod.workflow_instance(ctx.server.home, wfi)
    if (inst or {}).get("status") != "failed":
        return ("S22 max_fail_restarts", "FAIL",
                f"wfi status={inst.get('status') if inst else None}")
    sessions = db_mod.agent_sessions_for_instance(ctx.server.home, wfi)
    if len(sessions) != 3:
        return ("S22 max_fail_restarts", "FAIL",
                f"session count = {len(sessions)}, want 3")
    return ("S22 max_fail_restarts", "PASS",
            f"3 attempts (initial + 2 restarts)")
