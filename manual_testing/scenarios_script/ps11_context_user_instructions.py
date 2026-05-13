"""PS11 — script: context() / user_instructions() / callback_info() /
previous_data().

Tests SDK methods: `c.context()`, `c.user_instructions()`,
`c.callback_info()`, `c.previous_data()`.

Strategy: run the workflow with a known instructions string, have the
script record what `c.context()` returns, then assert against the DB.
"""

from __future__ import annotations

from lib import db as db_mod
from lib.runtime import (
    Ctx, PASS_STATUSES, Result, first_session,
    make_project, next_id, wait_for_workflow,
)
from lib.script_helpers import make_script_agent


INSTRUCTIONS = "ps11-marker-instructions-7831"

CODE = """
ctx = c.context()
c.findings.add("user_instr", c.user_instructions())
c.findings.add("ctx_has_session_id", str("session_id" in ctx))
c.findings.add("ctx_has_project_id", str("project_id" in ctx))
c.findings.add("ctx_has_scope_type", str("scope_type" in ctx))
# No callback was triggered; callback_info() must be None.
c.findings.add("callback_info_none", str(c.callback_info() is None))
# No relaunch; previous_data() must be empty string.
c.findings.add("previous_data_empty", str(c.previous_data() == ""))
# Refresh path returns same dict on a no-op call.
ctx2 = c.context(refresh=True)
c.findings.add("refresh_matches", str(ctx2["session_id"] == ctx["session_id"]))
c.agent.finished()
"""


def run(ctx: Ctx) -> Result:
    pid, _root = make_project(ctx)
    wid = next_id(ctx, "wf")
    ctx.client.create_workflow(pid, wid, scope_type="project")
    make_script_agent(ctx, pid, wid, "main", code=CODE)
    wfi = ctx.client.run_project_workflow(
        pid, wid, instructions=INSTRUCTIONS,
    )["instance_id"]
    wait_for_workflow(ctx, pid, instance_id=wfi)

    sess = first_session(db_mod.agent_sessions_for_instance(ctx.server.home, wfi))
    if sess["status"] not in PASS_STATUSES or sess["result"] != "pass":
        return ("PS11 context()", "FAIL",
                f"status/result = {sess['status']}/{sess['result']}")
    f = sess.get("findings") or {}
    if f.get("user_instr") != INSTRUCTIONS:
        return ("PS11 context()", "FAIL",
                f"user_instr = {f.get('user_instr')!r}")
    for k in ("ctx_has_session_id", "ctx_has_project_id",
              "ctx_has_scope_type", "callback_info_none",
              "previous_data_empty", "refresh_matches"):
        if f.get(k) != "True":
            return ("PS11 context()", "FAIL", f"{k} = {f.get(k)!r}")
    return ("PS11 context()", "PASS",
            f"context dict + user_instructions + previous_data all correct")
