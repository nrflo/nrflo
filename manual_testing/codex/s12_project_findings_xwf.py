"""S12 — Project findings shared across workflows.

Tests:
  - Workflow A writes a project-scope finding via `findings project-add`.
  - Workflow B's prompt uses `#{PROJECT_FINDINGS:favorite_color}`; the
    spawner substitutes the live value from project_findings at render
    time.
  - Workflow B then writes the resolved value back into its own findings.

Expected PASS result:
  - WF_B agent_sessions.prompt contains 'blue'.
  - WF_B agent_sessions.findings.observed_color == 'blue'.
"""

from __future__ import annotations

from lib import db as db_mod
from lib.runtime import (
    Ctx, Result, first_session, make_project, next_id, resolve_model, wait_for_workflow,
)


# Per-provider model overrides; empty = use the runner default (e.g. haiku).
MODELS_BY_PROVIDER: dict[str, str] = {}


WRITER_PROMPT = """\
You are an integration-test agent. Do EXACTLY what is listed below and
nothing else. Use the Bash tool to run the listed commands in order,
then stop immediately.

1. Run: `nrflo findings project-add favorite_color blue`
2. Run: `nrflo agent finished`
"""

READER_PROMPT = """\
You are an integration-test agent. Do EXACTLY what is listed below and
nothing else. Use the Bash tool to run the listed commands in order,
then stop immediately.

1. Run: `nrflo findings add observed_color #{PROJECT_FINDINGS:favorite_color}`
2. Run: `nrflo agent finished`
"""


def run(ctx: Ctx) -> Result:
    pid, _root = make_project(ctx)

    wa = next_id(ctx, "wf")
    ctx.client.create_workflow(pid, wa, scope_type="project")
    ctx.client.create_agent_def(
        pid, wa, "writer",
        model=resolve_model(ctx, MODELS_BY_PROVIDER), layer=0, timeout=5, prompt=WRITER_PROMPT,
    )
    wfi_a = ctx.client.run_project_workflow(
        pid, wa, instructions="write project finding",
    )["instance_id"]
    wait_for_workflow(ctx, pid, instance_id=wfi_a)

    wb = next_id(ctx, "wf")
    ctx.client.create_workflow(pid, wb, scope_type="project")
    ctx.client.create_agent_def(
        pid, wb, "reader",
        model=resolve_model(ctx, MODELS_BY_PROVIDER), layer=0, timeout=5, prompt=READER_PROMPT,
    )
    wfi_b = ctx.client.run_project_workflow(
        pid, wb, instructions="read project finding",
    )["instance_id"]
    wait_for_workflow(ctx, pid, instance_id=wfi_b)

    sess = first_session(
        db_mod.agent_sessions_for_instance(ctx.server.home, wfi_b))
    if "blue" not in (sess.get("prompt") or ""):
        return ("S12 project findings xwf", "FAIL",
                "value did not expand into WF_B prompt")
    obs = (sess.get("findings") or {}).get("observed_color")
    if obs != "blue":
        return ("S12 project findings xwf", "FAIL",
                f"observed_color = {obs!r}")
    return ("S12 project findings xwf", "PASS",
            "WF_B read WF_A's project finding")
