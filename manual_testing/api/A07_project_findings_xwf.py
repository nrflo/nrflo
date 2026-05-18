"""A07 — api-mode project_findings persist across workflows.

WF_A writes a project-scope finding via the `project_findings_add`
builtin. WF_B's prompt uses `#{PROJECT_FINDINGS:favorite_color}` and
writes the resolved value back into its own session findings via
`findings_add`. Verifies both that the in-process builtin writes to the
same `project_findings` table the CLI socket-method writes to, and that
template expansion still works for api-mode agents.

Expected PASS:
  - WF_B agent_sessions.prompt contains 'blue'.
  - WF_B agent_sessions.findings.observed_color == 'blue'.
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
You are an integration-test agent in api-mode. Call these tools in order:
  1. `findings_add` with {"key": "observed_color", "value": "#{PROJECT_FINDINGS:favorite_color}"}
  2. `agent_finished` with {}
Then stop.

Note: the template `#{PROJECT_FINDINGS:favorite_color}` was already
expanded by the spawner before this prompt reached you; treat it as a
literal value to forward into `findings_add`.
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
        tools="findings_add,agent_finished",
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
    obs = (sess.get("findings") or {}).get("observed_color")
    if obs != "blue":
        return ("A07 project findings xwf", "FAIL",
                f"observed_color = {obs!r}")
    return ("A07 project findings xwf", "PASS",
            "WF_B read WF_A's project finding")
