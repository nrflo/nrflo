"""PS01 — script: findings.add + agent.finished (own-session basics).

Tests SDK methods: `c.findings.add`, `c.agent.finished`.

Expected PASS:
  - agent_sessions.status ∈ {completed, project_completed}
  - agent_sessions.result == 'pass'
  - agent_sessions.findings == {"greeting": "hello"}
"""

from __future__ import annotations

from lib import db as db_mod
from lib.runtime import (
    Ctx, PASS_STATUSES, Result, first_session,
    make_project, next_id, wait_for_workflow,
)
from lib.script_helpers import make_script_agent


CODE = """
c.findings.add("greeting", "hello")
c.agent.finished()
"""


def run(ctx: Ctx) -> Result:
    pid, _root = make_project(ctx)
    wid = next_id(ctx, "wf")
    ctx.client.create_workflow(pid, wid, scope_type="project")
    make_script_agent(ctx, pid, wid, "main", code=CODE)
    wfi = ctx.client.run_project_workflow(
        pid, wid, instructions="ps01",
    )["instance_id"]
    wait_for_workflow(ctx, pid, instance_id=wfi)

    sess = first_session(db_mod.agent_sessions_for_instance(ctx.server.home, wfi))
    if sess["status"] not in PASS_STATUSES or sess["result"] != "pass":
        return ("PS01 findings.add", "FAIL",
                f"status/result = {sess['status']}/{sess['result']}")
    if (sess.get("findings") or {}).get("greeting") != "hello":
        return ("PS01 findings.add", "FAIL",
                f"findings = {sess.get('findings')!r}")
    return ("PS01 findings.add", "PASS", f"session={sess['id']}")
