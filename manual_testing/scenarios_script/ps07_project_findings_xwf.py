"""PS07 — script: project_findings persist across workflows in the same project.

Two workflows in one project. Wf1 writes a project finding; Wf2 reads
it back and records what it saw into its own session findings.
"""

from __future__ import annotations

from lib import db as db_mod
from lib.runtime import (
    Ctx, PASS_STATUSES, Result, first_session,
    make_project, next_id, wait_for_workflow,
)
from lib.script_helpers import make_script_agent


WRITER_CODE = """
c.project_findings.add("shared", "value-from-wf1")
c.agent.finished()
"""

READER_CODE = """
got = c.project_findings.get(key="shared")
c.findings.add("readback", str(got))
c.agent.finished()
"""


def run(ctx: Ctx) -> Result:
    pid, _root = make_project(ctx)

    wid1 = next_id(ctx, "wf")
    ctx.client.create_workflow(pid, wid1, scope_type="project")
    make_script_agent(ctx, pid, wid1, "main", code=WRITER_CODE)
    wfi1 = ctx.client.run_project_workflow(
        pid, wid1, instructions="ps07-write",
    )["instance_id"]
    wait_for_workflow(ctx, pid, instance_id=wfi1)

    wid2 = next_id(ctx, "wf")
    ctx.client.create_workflow(pid, wid2, scope_type="project")
    make_script_agent(ctx, pid, wid2, "main", code=READER_CODE)
    wfi2 = ctx.client.run_project_workflow(
        pid, wid2, instructions="ps07-read",
    )["instance_id"]
    wait_for_workflow(ctx, pid, instance_id=wfi2)

    sess = first_session(db_mod.agent_sessions_for_instance(ctx.server.home, wfi2))
    if sess["status"] not in PASS_STATUSES or sess["result"] != "pass":
        return ("PS07 project_findings xwf", "FAIL",
                f"reader status/result = {sess['status']}/{sess['result']}")
    rb = (sess.get("findings") or {}).get("readback", "")
    if "value-from-wf1" not in rb:
        return ("PS07 project_findings xwf", "FAIL",
                f"readback={rb!r}")
    return ("PS07 project_findings xwf", "PASS",
            f"reader saw shared finding across workflows")
