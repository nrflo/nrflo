"""S03 — Project-level findings.

Tests:
  - `nrflo findings project-add <k> <v>` writes to project_findings table.
  - Values are stored JSON-encoded; helper decodes back to a plain value.

Expected PASS result:
  - project_findings.team == 'alpha' (after JSON decode)
"""

from __future__ import annotations

from lib import db as db_mod
from lib.runtime import Ctx, Result, make_project, next_id, resolve_model, wait_for_workflow


# Per-provider model overrides; empty = use the runner default (e.g. haiku).
MODELS_BY_PROVIDER: dict[str, str] = {}


PROMPT = """\
You are an integration-test agent. Do EXACTLY what is listed below and
nothing else. Use the Bash tool to run the listed commands in order,
then stop immediately.

1. Run: `nrflo findings project-add team alpha`
2. Run: `nrflo agent finished`
"""


def run(ctx: Ctx) -> Result:
    pid, _root = make_project(ctx)
    wid = next_id(ctx, "wf")
    ctx.client.create_workflow(pid, wid, scope_type="project")
    ctx.client.create_agent_def(
        pid, wid, "main",
        model=resolve_model(ctx, MODELS_BY_PROVIDER), layer=0, timeout=5, prompt=PROMPT,
    )
    wfi = ctx.client.run_project_workflow(
        pid, wid, instructions="project findings",
    )["instance_id"]
    wait_for_workflow(ctx, pid, instance_id=wfi)

    pf = db_mod.project_findings(ctx.server.home, pid)
    if pf.get("team") != "alpha":
        return ("S03 project findings", "FAIL",
                f"project_findings.team = {pf.get('team')!r}, want 'alpha'")
    return ("S03 project findings", "PASS", f"keys={list(pf)}")
