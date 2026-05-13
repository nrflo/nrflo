"""PS04 — script: findings.delete(*keys).

Tests SDK method: `c.findings.delete(...)`.

Strategy: write three keys, delete two, finish. Verify the remaining
key is the only one in the session findings.
"""

from __future__ import annotations

from lib import db as db_mod
from lib.runtime import (
    Ctx, PASS_STATUSES, Result, first_session,
    make_project, next_id, wait_for_workflow,
)
from lib.script_helpers import make_script_agent


CODE = """
c.findings.add_bulk({"keep": "yes", "drop_a": "1", "drop_b": "2"})
c.findings.delete("drop_a", "drop_b")
c.agent.finished()
"""


def run(ctx: Ctx) -> Result:
    pid, _root = make_project(ctx)
    wid = next_id(ctx, "wf")
    ctx.client.create_workflow(pid, wid, scope_type="project")
    make_script_agent(ctx, pid, wid, "main", code=CODE)
    wfi = ctx.client.run_project_workflow(
        pid, wid, instructions="ps04",
    )["instance_id"]
    wait_for_workflow(ctx, pid, instance_id=wfi)

    sess = first_session(db_mod.agent_sessions_for_instance(ctx.server.home, wfi))
    if sess["status"] not in PASS_STATUSES or sess["result"] != "pass":
        return ("PS04 findings.delete", "FAIL",
                f"status/result = {sess['status']}/{sess['result']}")
    f = sess.get("findings") or {}
    if f.get("keep") != "yes":
        return ("PS04 findings.delete", "FAIL", f"keep missing: {f}")
    if "drop_a" in f or "drop_b" in f:
        return ("PS04 findings.delete", "FAIL",
                f"drop_* still present: {f}")
    return ("PS04 findings.delete", "PASS", f"findings={f}")
