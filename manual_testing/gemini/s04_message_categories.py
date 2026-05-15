"""S04 — Message categories (text + tool).

Tests:
  - The spawner records agent stream events into agent_messages with a
    `category` column.
  - A Bash invocation results in at least one `category='tool'` row;
    plain text output is `category='text'`.

Expected PASS result:
  - agent_messages for the session contains at least one category='tool' row.
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
nothing else. Use the Bash tool to run the listed commands in order,
then stop immediately.

1. Run: `echo hello-from-bash`
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
        pid, wid, instructions="message categories",
    )["instance_id"]
    wait_for_workflow(ctx, pid, instance_id=wfi)

    sess = first_session(db_mod.agent_sessions_for_instance(ctx.server.home, wfi))
    msgs = db_mod.agent_messages(ctx.server.home, sess["id"])
    cats = {m["category"] for m in msgs}
    if "tool" not in cats:
        return ("S04 message categories", "FAIL",
                f"no tool-category message recorded (saw {cats})")
    return ("S04 message categories", "PASS",
            f"categories={sorted(cats)}, msgs={len(msgs)}")
