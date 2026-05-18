"""A01 — api-mode hello world: end_turn-equivalent via `agent_finished`.

Exercises the apirun loop end-to-end:
  - `Runner.Run` issues an Anthropic streaming request,
  - the model calls the in-process `agent_finished` builtin,
  - the `TerminalSignal{Status:"PASS"}` short-circuits the loop,
  - the spawner maps the signal to result=pass / reason=implicit.

Expected PASS:
  - agent_sessions.effective_mode == 'api'
  - agent_sessions.status ∈ {completed, project_completed}
  - agent_sessions.result == 'pass'
"""

from __future__ import annotations

from lib import db as db_mod
from lib.runtime import (
    Ctx, PASS_STATUSES, Result,
    first_session, make_project, next_id, resolve_model, wait_for_workflow,
)


MODELS_BY_PROVIDER: dict[str, str] = {}

PROMPT = """\
You are an integration-test agent running in api-mode. You have one
available tool: `agent_finished`. Call it once with no arguments to mark
this agent as successfully finished. Do not emit any other text or tool
call before invoking the tool.
"""


def run(ctx: Ctx) -> Result:
    pid, _root = make_project(ctx)
    wid = next_id(ctx, "wf")
    ctx.client.create_workflow(pid, wid, scope_type="project")
    ctx.client.create_agent_def(
        pid, wid, "main",
        model=resolve_model(ctx, MODELS_BY_PROVIDER),
        layer=0, timeout=60, prompt=PROMPT,
        tools="agent_finished",
    )
    wfi = ctx.client.run_project_workflow(
        pid, wid, instructions="api-mode hello",
    )["instance_id"]
    wait_for_workflow(ctx, pid, instance_id=wfi)

    sess = first_session(db_mod.agent_sessions_for_instance(ctx.server.home, wfi))
    if sess.get("effective_mode") != "api":
        return ("A01 hello world", "FAIL",
                f"effective_mode = {sess.get('effective_mode')!r}, want 'api'")
    if sess["status"] not in PASS_STATUSES or sess["result"] != "pass":
        return ("A01 hello world", "FAIL",
                f"status/result = {sess['status']}/{sess['result']}")
    return ("A01 hello world", "PASS", f"session={sess['id']}")
