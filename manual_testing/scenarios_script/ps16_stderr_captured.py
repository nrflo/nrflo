"""PS16 — script: stderr lines surface as [stderr]-prefixed messages.

`backend_script.go` reads stderr per-line and pushes each as a text
message with `[stderr] ` prefix into `agent_messages`. Asserts the
prefix landed.
"""

from __future__ import annotations

from lib import db as db_mod
from lib.runtime import (
    Ctx, PASS_STATUSES, Result, first_session,
    make_project, next_id, wait_for_workflow,
)
from lib.script_helpers import make_script_agent


MARKER = "ps16-marker-on-stderr"

CODE = f"""
import sys
print({MARKER!r}, file=sys.stderr)
c.agent.finished()
"""


def run(ctx: Ctx) -> Result:
    pid, _root = make_project(ctx)
    wid = next_id(ctx, "wf")
    ctx.client.create_workflow(pid, wid, scope_type="project")
    make_script_agent(ctx, pid, wid, "main", code=CODE)
    wfi = ctx.client.run_project_workflow(
        pid, wid, instructions="ps16",
    )["instance_id"]
    wait_for_workflow(ctx, pid, instance_id=wfi)

    sess = first_session(db_mod.agent_sessions_for_instance(ctx.server.home, wfi))
    if sess["status"] not in PASS_STATUSES or sess["result"] != "pass":
        return ("PS16 stderr captured", "FAIL",
                f"status/result = {sess['status']}/{sess['result']}")
    msgs = db_mod.agent_messages(ctx.server.home, sess["id"])
    hit = next((m for m in msgs
                if MARKER in (m.get("content") or "")
                and (m.get("content") or "").startswith("[stderr]")), None)
    if not hit:
        contents = [m.get("content") for m in msgs][:10]
        return ("PS16 stderr captured", "FAIL",
                f"no [stderr] message containing {MARKER!r}; "
                f"first 10 msgs: {contents}")
    return ("PS16 stderr captured", "PASS",
            f"stderr line captured: {hit['content']!r}")
