"""S05 — context_left populated.

Tests:
  - The CLI integration reports remaining context % to the server via
    `agent.context_update`, populating agent_sessions.context_left.

Expected result:
  - PASS  agent_sessions.context_left ∈ [0, 100]
  - SKIP  if NULL (some CLIs/modes don't ship context telemetry)
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
nothing else. Use the Bash tool to run the listed command, then stop.

1. Run: `nrflo agent finished`
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
        pid, wid, instructions="context left",
    )["instance_id"]
    wait_for_workflow(ctx, pid, instance_id=wfi)

    sess = first_session(db_mod.agent_sessions_for_instance(ctx.server.home, wfi))
    cl = sess.get("context_left")
    if cl is None:
        return ("S05 context_left", "SKIP",
                f"{ctx.provider}/{ctx.mode} did not populate context_left")
    if not (0 <= cl <= 100):
        return ("S05 context_left", "FAIL", f"context_left out of range: {cl}")
    return ("S05 context_left", "PASS", f"context_left={cl}")
