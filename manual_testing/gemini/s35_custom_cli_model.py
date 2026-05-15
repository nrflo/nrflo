"""S35 — Custom cli_models row resolves to a real CLI binary.

Tests:
  - `POST /api/v1/cli-models` with a brand-new ID + cli_type=claude
    inserts the row. An agent_def using that ID must resolve through
    `spawner.cliForModel` to the claude binary and complete normally.
  - claude only — the scenario asserts the resolution path for one
    provider; codex/opencode share the same code (validated by their
    seed cli_models rows already exercised by every other scenario).
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
    if ctx.provider != "claude":
        return ("S35 custom cli_model", "SKIP",
                f"{ctx.provider} — claude only by design")

    pid, _root = make_project(ctx)
    model_id = next_id(ctx, "cm-haiku")
    ctx.client.create_cli_model(
        id=model_id,
        cli_type="claude",
        display_name=f"Custom Haiku ({model_id})",
        mapped_model="haiku",
        context_length=200000,
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
        return ("S35 custom cli_model", "FAIL",
                f"session status/result = {sess['status']}/{sess['result']}")
    # The spawner stores model_id as "<cli_type>:<id>" (spawner.go:898).
    sess_model = sess.get("model_id") or ""
    if not sess_model.endswith(model_id):
        return ("S35 custom cli_model", "FAIL",
                f"agent_sessions.model_id = {sess_model!r}, "
                f"want suffix {model_id!r}")
    return ("S35 custom cli_model", "PASS",
            f"resolved {sess_model} → claude/haiku, session={sess['id'][:8]}")
