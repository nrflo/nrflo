"""A07 — api-mode project_findings persist across workflows.

WF_A writes a project-scope finding via the `project_findings_add`
builtin. WF_B's prompt uses `#{PROJECT_FINDINGS:favorite_color}`; the
spawner expands it at render time. Verifies both that the in-process
builtin writes a row queryable by the spawner's template engine, and
that template expansion fires for api-mode agents (same path as CLI).

Expected PASS:
  - WF_B agent_sessions.prompt contains 'blue'.
"""

from __future__ import annotations

from lib import db as db_mod
from lib.runtime import (
    Ctx, Result, first_session, make_project, next_id, resolve_model, wait_for_workflow,
)


MODELS_BY_PROVIDER: dict[str, str] = {}

WRITER_PROMPT = """\
You are an integration-test agent in api-mode. Call these tools in order:
  1. `project_findings_add` with {"key": "favorite_color", "value": "blue"}
  2. `agent_finished` with {}
Then stop.
"""

READER_PROMPT = """\
You are an integration-test agent in api-mode. The favourite color
is: #{PROJECT_FINDINGS:favorite_color}. Call `agent_finished` with {}
and stop.
"""


def run(ctx: Ctx) -> Result:
    pid, _root = make_project(ctx)

    wa = next_id(ctx, "wf")
    ctx.client.create_workflow(pid, wa, scope_type="project")
    ctx.client.create_agent_def(
        pid, wa, "writer",
        model=resolve_model(ctx, MODELS_BY_PROVIDER),
        layer=0, timeout=60, prompt=WRITER_PROMPT,
        tools="project_findings_add,agent_finished",
    )
    wfi_a = ctx.client.run_project_workflow(
        pid, wa, instructions="write project finding",
    )["instance_id"]
    wait_for_workflow(ctx, pid, instance_id=wfi_a)

    wb = next_id(ctx, "wf")
    ctx.client.create_workflow(pid, wb, scope_type="project")
    ctx.client.create_agent_def(
        pid, wb, "reader",
        model=resolve_model(ctx, MODELS_BY_PROVIDER),
        layer=0, timeout=60, prompt=READER_PROMPT,
        tools="agent_finished",
    )
    wfi_b = ctx.client.run_project_workflow(
        pid, wb, instructions="read project finding",
    )["instance_id"]
    wait_for_workflow(ctx, pid, instance_id=wfi_b)

    sess = first_session(
        db_mod.agent_sessions_for_instance(ctx.server.home, wfi_b))
    if "blue" not in (sess.get("prompt") or ""):
        return ("A07 project findings xwf", "FAIL",
                "value did not expand into WF_B prompt")
    return ("A07 project findings xwf", "PASS",
            "WF_B read WF_A's project finding via template")
