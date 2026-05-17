"""S02 — Agent fail.

Tests:
  - `nrflo agent fail --reason "..."` marks the session failed.
  - An `errors` row of type 'agent' is recorded for the project.

Expected PASS result:
  - agent_sessions.result == 'fail'
  - agent_sessions.result_reason contains 'intentional'
  - at least one errors row with error_type='agent' for this project
"""

from __future__ import annotations

from lib import db as db_mod
from lib.runtime import (
    Ctx, Result, first_session, make_project, next_id, resolve_model, wait_for_workflow,
)


# Per-provider model overrides; empty = use the runner default (e.g. haiku).
MODELS_BY_PROVIDER: dict[str, str] = {}


PROMPT = """\
You are an integration-test agent. Do EXACTLY what is listed below and
nothing else. Run the command via the Bash tool, then stop immediately.

1. Run: `nrflo agent fail --reason "intentional"`
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
        pid, wid, instructions="run the test",
    )["instance_id"]
    wait_for_workflow(ctx, pid, instance_id=wfi)

    sess = first_session(db_mod.agent_sessions_for_instance(ctx.server.home, wfi))
    if sess["result"] != "fail":
        return ("S02 agent fail", "FAIL", f"result = {sess['result']!r}")
    if "intentional" not in (sess.get("result_reason") or ""):
        return ("S02 agent fail", "FAIL",
                f"result_reason = {sess.get('result_reason')!r}")
    errs = db_mod.errors_for_project(ctx.server.home, pid)
    if not any(e["error_type"] == "agent" for e in errs):
        return ("S02 agent fail", "FAIL",
                f"no agent error row (saw {errs})")
    return ("S02 agent fail", "PASS", f"session={sess['id']}")
