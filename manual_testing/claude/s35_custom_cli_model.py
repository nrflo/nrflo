"""S35 — A custom CLI-capable model resolves to the Claude binary.

Tests:
  - `POST /api/v1/models` inserts a brand-new anthropic row with CLI mode.
  - An agent definition using that slug resolves provider→claude and
    completes normally.
"""

from __future__ import annotations

from lib import db as db_mod
from lib.runtime import (
    Ctx, PASS_STATUSES, Result,
    first_session, make_project, next_id, wait_for_workflow,
)


PROMPT = """\
You are an integration-test agent. Run the listed commands via the Bash
tool, then stop.

1. Run: the `agent_finished` tool
"""


def run(ctx: Ctx) -> Result:
    pid, _root = make_project(ctx)
    model_id = next_id(ctx, "cm-haiku")
    ctx.client.create_model(
        id=model_id,
        provider="anthropic",
        display_name=f"Custom Haiku ({model_id})",
        cli_model="claude-haiku-4-5",
        cli_efforts=["low", "medium", "high"],
        cli_context=200000,
    )

    wid = next_id(ctx, "wf")
    ctx.client.create_workflow(pid, wid, scope_type="project")
    ctx.client.create_agent_def(
        pid, wid, "main",
        model=model_id, layer=0, timeout=5, prompt=PROMPT,
    )
    wfi = ctx.client.run_project_workflow(
        pid, wid, instructions="custom cli model",
    )["instance_id"]
    wait_for_workflow(ctx, pid, instance_id=wfi)

    sess = first_session(db_mod.agent_sessions_for_instance(ctx.server.home, wfi))
    if sess["status"] not in PASS_STATUSES or sess["result"] != "pass":
        return ("S35 custom model", "FAIL",
                f"session status/result = {sess['status']}/{sess['result']}")
    sess_model = sess.get("model_id") or ""
    if not sess_model.endswith(model_id):
        return ("S35 custom model", "FAIL",
                f"agent_sessions.model_id = {sess_model!r}, "
                f"want suffix {model_id!r}")
    return ("S35 custom model", "PASS",
            f"resolved {sess_model} → claude/claude-haiku-4-5, session={sess['id'][:8]}")
