"""PS13 — script: c.skip(tag).

Tests SDK method: `c.skip("flaky-step")`. The handler validates the
tag against workflow.groups, so the workflow MUST declare the tag in
its `groups` list at creation.
"""

from __future__ import annotations

from lib import db as db_mod
from lib.runtime import (
    Ctx, Result, make_project, next_id, wait_for_workflow,
)
from lib.script_helpers import make_script_agent


SKIP_TAG = "flaky-step"

CODE = f"""
c.skip({SKIP_TAG!r})
c.agent.finished()
"""


def run(ctx: Ctx) -> Result:
    pid, _root = make_project(ctx)
    wid = next_id(ctx, "wf")
    ctx.client.create_workflow(
        pid, wid, scope_type="project", groups=[SKIP_TAG])
    make_script_agent(ctx, pid, wid, "main", code=CODE)
    wfi = ctx.client.run_project_workflow(
        pid, wid, instructions="ps13",
    )["instance_id"]
    wait_for_workflow(ctx, pid, instance_id=wfi)

    inst = db_mod.workflow_instance(ctx.server.home, wfi)
    tags = (inst or {}).get("skip_tags")
    if not isinstance(tags, list) or SKIP_TAG not in tags:
        return ("PS13 c.skip", "FAIL", f"skip_tags = {tags!r}")
    return ("PS13 c.skip", "PASS", f"skip_tags={tags}")
