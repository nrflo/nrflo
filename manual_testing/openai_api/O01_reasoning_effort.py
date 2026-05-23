"""O01 — OpenAI api-mode: reasoning_effort seeded row drives a successful run.

Authors an agent on the `gpt54_high` api_models row (reasoning_effort='high').
Asserts via GET /api/v1/api-models/gpt54_high that reasoning_effort is
non-empty, then verifies the agent completes successfully.

Expected PASS:
  - api_models row gpt54_high has reasoning_effort == 'high'
  - agent_sessions.effective_mode == 'api'
  - agent_sessions.status ∈ PASS_STATUSES
  - agent_sessions.result == 'pass'
"""

from __future__ import annotations

from lib import db as db_mod
from lib.runtime import (
    Ctx, PASS_STATUSES, Result,
    first_session, make_project, next_id, wait_for_workflow,
)

PROMPT = """\
You are an integration-test agent running in api-mode. You have one
available tool: `agent_finished`. Call it once with no arguments to mark
this agent as successfully finished. Do not emit any other text or tool
call before invoking the tool.
"""


def run(ctx: Ctx) -> Result:
    am = ctx.client.get_api_model("gpt54_high")
    if am.get("reasoning_effort") != "high":
        return ("O01 reasoning effort", "FAIL",
                f"gpt54_high.reasoning_effort = {am.get('reasoning_effort')!r}, want 'high'")

    pid, _root = make_project(ctx)
    wid = next_id(ctx, "wf")
    ctx.client.create_workflow(pid, wid, scope_type="project")
    ctx.client.create_agent_def(
        pid, wid, "main",
        model="gpt54_high",
        layer=0, timeout=90, prompt=PROMPT,
        tools="agent_finished",
    )
    wfi = ctx.client.run_project_workflow(
        pid, wid, instructions="openai reasoning-effort test",
    )["instance_id"]
    wait_for_workflow(ctx, pid, instance_id=wfi)

    sess = first_session(db_mod.agent_sessions_for_instance(ctx.server.home, wfi))
    if sess.get("effective_mode") != "api":
        return ("O01 reasoning effort", "FAIL",
                f"effective_mode = {sess.get('effective_mode')!r}, want 'api'")
    if sess["status"] not in PASS_STATUSES or sess["result"] != "pass":
        return ("O01 reasoning effort", "FAIL",
                f"status/result = {sess['status']}/{sess['result']}")
    return ("O01 reasoning effort", "PASS", f"session={sess['id']}")
