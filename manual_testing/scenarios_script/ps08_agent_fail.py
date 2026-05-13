"""PS08 — script: agent.fail(reason="...").

Tests SDK method: `c.agent.fail("reason")`. The session row must record
result='fail' and the reason in result_reason.

Notes:
  - Without max_fail_restarts cap, the orchestrator would relaunch on
    failure; we set max_fail_restarts=0 to make the first failure
    terminal.
"""

from __future__ import annotations

from lib import db as db_mod
from lib.runtime import (
    Ctx, Result, make_project, next_id, wait_for_workflow,
)
from lib.script_helpers import make_script_agent


CODE = """
c.findings.add("attempted", "yes")
c.agent.fail("intentional ps08 failure")
"""


def run(ctx: Ctx) -> Result:
    pid, _root = make_project(ctx)
    wid = next_id(ctx, "wf")
    ctx.client.create_workflow(pid, wid, scope_type="project")
    make_script_agent(ctx, pid, wid, "main", code=CODE, max_fail_restarts=0)
    wfi = ctx.client.run_project_workflow(
        pid, wid, instructions="ps08",
    )["instance_id"]
    wait_for_workflow(ctx, pid, instance_id=wfi)

    sessions = db_mod.agent_sessions_for_instance(ctx.server.home, wfi)
    if not sessions:
        return ("PS08 agent.fail", "FAIL", "no sessions recorded")
    last = sessions[-1]
    if last.get("result") != "fail":
        return ("PS08 agent.fail", "FAIL",
                f"result = {last.get('result')!r}")
    reason = last.get("result_reason") or ""
    if "intentional ps08" not in reason:
        return ("PS08 agent.fail", "FAIL",
                f"result_reason = {reason!r}")
    return ("PS08 agent.fail", "PASS",
            f"result=fail reason={reason!r}")
