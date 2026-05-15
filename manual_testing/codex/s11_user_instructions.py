"""S11 — User instructions injected into agent prompt.

Tests:
  - The `instructions` field passed to POST .../workflow/run is expanded
    into the agent's rendered prompt via the ${user_instructions} variable.

Expected PASS result:
  - agent_sessions.prompt contains the unique marker we passed as
    instructions (proves the substitution happened server-side).
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
    marker = "UNIQ_S11_" + next_id(ctx, "m")
    pid, _root = make_project(ctx)
    wid = next_id(ctx, "wf")
    ctx.client.create_workflow(pid, wid, scope_type="project")
    ctx.client.create_agent_def(
        pid, wid, "main",
        model=resolve_model(ctx, MODELS_BY_PROVIDER), layer=0, timeout=5, prompt=PROMPT,
    )
    wfi = ctx.client.run_project_workflow(
        pid, wid, instructions=marker,
    )["instance_id"]
    wait_for_workflow(ctx, pid, instance_id=wfi)

    sess = first_session(db_mod.agent_sessions_for_instance(ctx.server.home, wfi))
    if marker not in (sess.get("prompt") or ""):
        return ("S11 user instructions", "FAIL",
                "marker not found in rendered prompt")
    return ("S11 user instructions", "PASS",
            f"marker {marker} present in prompt")
