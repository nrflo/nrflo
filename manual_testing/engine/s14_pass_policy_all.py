"""S14 — Layer pass_policy='all' enforces failure on partial success.

Tests:
  - A layer with two agents, one of which fails, configured with
    pass_policy='all', must mark the whole workflow as failed.

Expected PASS result:
  - workflow_instances.status == 'failed' (even though one agent passed).
"""

from __future__ import annotations

from lib import db as db_mod
from lib.runtime import Ctx, Result, make_project, next_id, resolve_model, wait_for_workflow


# Per-provider model overrides; empty = use the runner default (e.g. haiku).
MODELS_BY_PROVIDER: dict[str, str] = {}


GOOD_PROMPT = """\
You are an integration-test agent. Do EXACTLY what is listed below and
nothing else. Use the Bash tool to run the listed command, then stop.

1. Run: `nrflo agent finished`
"""

BAD_PROMPT = """\
You are an integration-test agent. Do EXACTLY what is listed below and
nothing else. Run the command via the Bash tool, then stop.

1. Run: `nrflo agent fail --reason "intentional s14"`
"""


def run(ctx: Ctx) -> Result:
    pid, _root = make_project(ctx)
    wid = next_id(ctx, "wf")
    ctx.client.create_workflow(pid, wid, scope_type="project")
    ctx.client.create_agent_def(
        pid, wid, "good",
        model=resolve_model(ctx, MODELS_BY_PROVIDER), layer=0, timeout=5, prompt=GOOD_PROMPT,
    )
    ctx.client.create_agent_def(
        pid, wid, "bad",
        model=resolve_model(ctx, MODELS_BY_PROVIDER), layer=0, timeout=5, prompt=BAD_PROMPT,
    )
    ctx.client.set_layer_policy(pid, wid, layer=0, pass_policy="all")

    wfi = ctx.client.run_project_workflow(
        pid, wid, instructions="policy=all test",
    )["instance_id"]
    wait_for_workflow(ctx, pid, instance_id=wfi)

    inst = db_mod.workflow_instance(ctx.server.home, wfi)
    status = (inst or {}).get("status")
    if status != "failed":
        return ("S14 pass_policy=all", "FAIL", f"workflow status={status!r}")
    return ("S14 pass_policy=all", "PASS",
            "policy=all enforced fail on partial success")
