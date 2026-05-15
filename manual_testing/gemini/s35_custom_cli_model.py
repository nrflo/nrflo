"""S35 — Custom cli_models row resolves to a real CLI binary (gemini).

Mirrors claude/s35: registers a brand-new cli_models row with
`cli_type=gemini` and a known gemini model name as `mapped_model`. The
spawner must resolve the agent_def through `spawner.cliForModel` to the
gemini binary and complete normally; `agent_sessions.model_id` is stored
as `<cli_type>:<id>` (spawner.go:898).
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

1. Run: `nrflo agent finished`
"""


def run(ctx: Ctx) -> Result:
    pid, _root = make_project(ctx)
    model_id = next_id(ctx, "cm-gemini-lite")
    ctx.client.create_cli_model(
        id=model_id,
        cli_type="gemini",
        display_name=f"Custom Gemini Lite ({model_id})",
        mapped_model="gemini-2.5-flash-lite",
        context_length=1000000,
    )

    wid = next_id(ctx, "wf")
    ctx.client.create_workflow(pid, wid, scope_type="project")
    ctx.client.create_agent_def(
        pid, wid, "main",
        model=model_id, layer=0, timeout=120, prompt=PROMPT,
    )
    wfi = ctx.client.run_project_workflow(
        pid, wid, instructions="custom cli model",
    )["instance_id"]
    wait_for_workflow(ctx, pid, instance_id=wfi)

    sess = first_session(db_mod.agent_sessions_for_instance(ctx.server.home, wfi))
    if sess["status"] not in PASS_STATUSES or sess["result"] != "pass":
        return ("S35 custom cli_model", "FAIL",
                f"session status/result = {sess['status']}/{sess['result']}")
    sess_model = sess.get("model_id") or ""
    if not sess_model.endswith(model_id):
        return ("S35 custom cli_model", "FAIL",
                f"agent_sessions.model_id = {sess_model!r}, "
                f"want suffix {model_id!r}")
    return ("S35 custom cli_model", "PASS",
            f"resolved {sess_model} → gemini/flash-lite, session={sess['id'][:8]}")
