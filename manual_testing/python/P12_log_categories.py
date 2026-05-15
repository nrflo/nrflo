"""PS12 — script: c.log(type, message, payload).

Tests SDK method: `c.log(type, message, payload)`. Accepted categories
per the socket handler / SDK docstring: text, tool, subagent, skill,
user_input, error, result.

Strategy: emit one log per category; assert each row appears in
`agent_messages` with the expected category.
"""

from __future__ import annotations

from lib import db as db_mod
from lib.runtime import (
    Ctx, PASS_STATUSES, Result, first_session,
    make_project, next_id, wait_for_workflow,
)
from lib.script_helpers import make_script_agent


CATEGORIES = ["text", "tool", "subagent", "skill", "user_input", "error", "result"]

CODE = f"""
import json
cats = {CATEGORIES!r}
for cat in cats:
    c.log(cat, "marker-" + cat, payload={{"k": cat}})
c.agent.finished()
"""


def run(ctx: Ctx) -> Result:
    pid, _root = make_project(ctx)
    wid = next_id(ctx, "wf")
    ctx.client.create_workflow(pid, wid, scope_type="project")
    make_script_agent(ctx, pid, wid, "main", code=CODE)
    wfi = ctx.client.run_project_workflow(
        pid, wid, instructions="ps12",
    )["instance_id"]
    wait_for_workflow(ctx, pid, instance_id=wfi)

    sess = first_session(db_mod.agent_sessions_for_instance(ctx.server.home, wfi))
    if sess["status"] not in PASS_STATUSES or sess["result"] != "pass":
        return ("PS12 c.log categories", "FAIL",
                f"status/result = {sess['status']}/{sess['result']}")

    msgs = db_mod.agent_messages(ctx.server.home, sess["id"])
    found: dict[str, dict] = {}
    for m in msgs:
        if m.get("content", "").startswith("marker-"):
            cat = m["content"].removeprefix("marker-")
            found[cat] = m
    missing = [c for c in CATEGORIES if c not in found]
    if missing:
        return ("PS12 c.log categories", "FAIL",
                f"missing categories: {missing}; saw: {sorted(found)}")
    for cat, m in found.items():
        if m.get("category") != cat:
            return ("PS12 c.log categories", "FAIL",
                    f"category mismatch for {cat!r}: row={m}")
    return ("PS12 c.log categories", "PASS",
            f"all {len(CATEGORIES)} categories emitted + classified")
