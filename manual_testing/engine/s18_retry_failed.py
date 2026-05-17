"""S18 — Manual retry-failed re-spawns the failed agent.

Tests:
  - Run a workflow whose only agent calls `nrflo agent fail`.
  - POST /api/v1/projects/{id}/workflow/retry-failed with the failed
    session_id; the orchestrator must spawn a fresh agent_session on
    the same workflow_instance.
  - The new session will fail again (same prompt) — we only assert
    that retry caused a new spawn.

Expected PASS result:
  - At least one more agent_sessions row exists for the wfi after the
    retry call than before it.
"""

from __future__ import annotations

import time

from lib import db as db_mod
from lib.runtime import Ctx, Result, make_project, next_id, resolve_model, wait_for_workflow


# Per-provider model overrides; empty = use the runner default (e.g. haiku).
MODELS_BY_PROVIDER: dict[str, str] = {}


PROMPT = """\
You are an integration-test agent. Do EXACTLY what is listed below and
nothing else. Run the command via the Bash tool, then stop.

1. Run: `nrflo agent fail --reason "first try"`
"""


def run(ctx: Ctx) -> Result:
    pid, _root = make_project(ctx)
    wid = next_id(ctx, "wf")
    ctx.client.create_workflow(pid, wid, scope_type="project")
    ctx.client.create_agent_def(
        pid, wid, "main",
        model=resolve_model(ctx, MODELS_BY_PROVIDER), layer=0, timeout=5, prompt=PROMPT,
    )
    wfi = ctx.client.run_project_workflow(
        pid, wid, instructions="retry test",
    )["instance_id"]
    wait_for_workflow(ctx, pid, instance_id=wfi)

    before = db_mod.agent_sessions_for_instance(ctx.server.home, wfi)
    if not before or before[0].get("result") != "fail":
        return ("S18 retry-failed", "FAIL", "setup: first attempt did not fail")

    ctx.client.retry_failed_project(
        pid, instance_id=wfi, workflow=wid, session_id=before[0]["id"])

    deadline = time.monotonic() + 60.0
    while time.monotonic() < deadline:
        cur = db_mod.agent_sessions_for_instance(ctx.server.home, wfi)
        if len(cur) > len(before):
            break
        time.sleep(1)
    cur = db_mod.agent_sessions_for_instance(ctx.server.home, wfi)
    if len(cur) <= len(before):
        return ("S18 retry-failed", "FAIL",
                f"no new session (before={len(before)} after={len(cur)})")
    wait_for_workflow(ctx, pid, instance_id=wfi)
    return ("S18 retry-failed", "PASS",
            f"retry spawned new session (total={len(cur)})")
