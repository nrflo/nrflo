"""PS17 — script: multiple c.skip(tag) calls accumulate + de-duplicate.

Tests:
  - Four `c.skip(...)` calls with three distinct tags (one repeated)
    land in `workflow_instances.skip_tags` as a 3-entry JSON array.
  - Server de-duplicates: the repeated tag appears only once.
"""

from __future__ import annotations

from lib import db as db_mod
from lib.runtime import (
    Ctx, Result, make_project, next_id, wait_for_workflow,
)
from lib.script_helpers import make_script_agent


TAGS = ["tag-a", "tag-b", "tag-c"]

CODE = """
c.skip("tag-a")
c.skip("tag-b")
c.skip("tag-a")
c.skip("tag-c")
c.agent.finished()
"""


def run(ctx: Ctx) -> Result:
    pid, _root = make_project(ctx)
    wid = next_id(ctx, "wf")
    ctx.client.create_workflow(pid, wid, scope_type="project", groups=TAGS)
    make_script_agent(ctx, pid, wid, "main", code=CODE)
    wfi = ctx.client.run_project_workflow(
        pid, wid, instructions="ps17",
    )["instance_id"]
    wait_for_workflow(ctx, pid, instance_id=wfi)

    inst = db_mod.workflow_instance(ctx.server.home, wfi)
    tags = (inst or {}).get("skip_tags")
    if not isinstance(tags, list):
        return ("PS17 multi skip tag", "FAIL", f"skip_tags = {tags!r}")
    got = sorted(tags)
    want = sorted(TAGS)
    if got != want:
        return ("PS17 multi skip tag", "FAIL",
                f"skip_tags={got!r}, want={want!r} (de-duplicated)")
    return ("PS17 multi skip tag", "PASS", f"skip_tags={got}")
