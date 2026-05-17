"""S01 — Findings save (own-session).

Tests:
  - `nrflo findings add <k> <v>` writes to agent_sessions.findings JSON.
  - `nrflo agent finished` marks the session as a pass.

Expected PASS result:
  - agent_sessions.status ∈ {completed, project_completed}
  - agent_sessions.result == 'pass'
  - agent_sessions.findings.greeting == 'hello'
"""

from __future__ import annotations

from lib import db as db_mod
from lib.runtime import (
    Ctx, PASS_STATUSES, Result,
    first_session, make_project, next_id, resolve_model, wait_for_workflow,
)


# Per-provider model overrides; empty = use the runner default (e.g. haiku).
MODELS_BY_PROVIDER: dict[str, str] = {}

PROMPT = """\
You are an integration-test agent. Do EXACTLY what is listed below and
nothing else. Use the Bash tool to run the listed commands in order,
then stop immediately.

1. Run: `nrflo findings add greeting hello`
2. Run: `nrflo agent finished`
"""


def run(ctx: Ctx) -> Result:
    pid, _root = make_project(ctx)
    wid = next_id(ctx, "wf")
    ctx.client.create_workflow(pid, wid, scope_type="project")
    ctx.client.create_agent_def(
        pid, wid, "main",
        model=resolve_model(ctx, MODELS_BY_PROVIDER),
        layer=0, timeout=5, prompt=PROMPT,
    )
    wfi = ctx.client.run_project_workflow(
        pid, wid, instructions="run the test",
    )["instance_id"]
    wait_for_workflow(ctx, pid, instance_id=wfi)

    sess = first_session(db_mod.agent_sessions_for_instance(ctx.server.home, wfi))
    if sess["status"] not in PASS_STATUSES or sess["result"] != "pass":
        return ("S01 findings save", "FAIL",
                f"session status/result = {sess['status']}/{sess['result']}")
    greeting = (sess.get("findings") or {}).get("greeting")
    if greeting != "hello":
        return ("S01 findings save", "FAIL",
                f"findings.greeting = {greeting!r}")
    return ("S01 findings save", "PASS", f"session={sess['id']}")
