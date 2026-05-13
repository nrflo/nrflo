"""PS15 — script: uncaught exception → non-zero exit → result=fail.

The script raises before calling agent.finished/fail. backend_script's
wait goroutine records waitErr; the spawner finalizes the session as
fail and the orchestrator does not relaunch when max_fail_restarts=0.
"""

from __future__ import annotations

from lib import db as db_mod
from lib.runtime import (
    Ctx, Result, make_project, next_id, wait_for_workflow,
)
from lib.script_helpers import make_script_agent


CODE = """
c.findings.add("phase", "before-crash")
raise RuntimeError("intentional ps15 crash")
"""


def run(ctx: Ctx) -> Result:
    pid, _root = make_project(ctx)
    wid = next_id(ctx, "wf")
    ctx.client.create_workflow(pid, wid, scope_type="project")
    make_script_agent(ctx, pid, wid, "main", code=CODE, max_fail_restarts=0)
    wfi = ctx.client.run_project_workflow(
        pid, wid, instructions="ps15",
    )["instance_id"]
    wait_for_workflow(ctx, pid, instance_id=wfi)

    sessions = db_mod.agent_sessions_for_instance(ctx.server.home, wfi)
    if not sessions:
        return ("PS15 exception failure", "FAIL", "no sessions recorded")
    last = sessions[-1]
    if last.get("result") != "fail":
        return ("PS15 exception failure", "FAIL",
                f"result = {last.get('result')!r}, want fail")
    # findings.add ran before raise — the row should persist.
    if (last.get("findings") or {}).get("phase") != "before-crash":
        return ("PS15 exception failure", "FAIL",
                f"pre-crash finding lost: {last.get('findings')!r}")
    return ("PS15 exception failure", "PASS",
            f"result=fail recorded after uncaught exception")
