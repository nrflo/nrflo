"""S06 — Workflow skip tag.

Tests:
  - `nrflo skip <tag>` adds the tag to workflow_instances.skip_tags.
  - The socket handler validates the tag against workflow.groups —
    so the workflow MUST be created with groups=["flaky-step"].

Expected PASS result:
  - workflow_instances.skip_tags JSON contains "flaky-step".
"""

from __future__ import annotations

from lib import db as db_mod
from lib.runtime import (
    Ctx, Result, make_project, next_id, resolve_model, wait_for_workflow,
)


# Per-provider model overrides; empty = use the runner default (e.g. haiku).
MODELS_BY_PROVIDER: dict[str, str] = {}


SKIP_TAG = "flaky-step"

PROMPT = """\
You are an integration-test agent. Do EXACTLY what is listed below and
nothing else. Use the Bash tool to run the listed commands in order,
then stop immediately.

1. Run: `nrflo skip flaky-step`
2. Run: `nrflo agent finished`
"""


def run(ctx: Ctx) -> Result:
    pid, _root = make_project(ctx)
    wid = next_id(ctx, "wf")
    ctx.client.create_workflow(
        pid, wid, scope_type="project", groups=[SKIP_TAG])
    ctx.client.create_agent_def(
        pid, wid, "main",
        model=resolve_model(ctx, MODELS_BY_PROVIDER), layer=0, timeout=5, prompt=PROMPT,
    )
    wfi = ctx.client.run_project_workflow(
        pid, wid, instructions="skip test",
    )["instance_id"]
    wait_for_workflow(ctx, pid, instance_id=wfi)

    inst = db_mod.workflow_instance(ctx.server.home, wfi)
    tags = (inst or {}).get("skip_tags")
    if not isinstance(tags, list) or SKIP_TAG not in tags:
        return ("S06 skip tag", "FAIL", f"skip_tags = {tags!r}")
    return ("S06 skip tag", "PASS", f"skip_tags={tags}")
