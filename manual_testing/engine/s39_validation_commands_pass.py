"""S39 — Agent passes, declarative validation_commands run successfully.

Tests:
  - `validation_commands=["true"]` registered on the agent_def is invoked
    by the spawner after the agent self-reports pass
    (be/internal/spawner/completion.go:129).
  - Zero-exit validation leaves the session as pass; spawner records
    progress as `category='validation'` agent_messages rows.

Expected PASS:
  - agent_sessions.status ∈ {completed, project_completed}
  - agent_sessions.result == 'pass'
  - ≥1 agent_messages row with category='validation'
  - findings.validation_failure must be absent
"""

from __future__ import annotations

from lib import db as db_mod
from lib.runtime import (
    Ctx, PASS_STATUSES, Result,
    first_session, make_project, next_id, resolve_model, wait_for_workflow,
)


MODELS_BY_PROVIDER: dict[str, str] = {}

PROMPT = """\
You are an integration-test agent. Run the listed command via the Bash
tool, then stop.

1. Run: `nrflo agent finished`
"""


def run(ctx: Ctx) -> Result:
    pid, _root = make_project(ctx)
    wid = next_id(ctx, "wf")
    ctx.client.create_workflow(pid, wid, scope_type="project")
    ctx.client.create_agent_def(
        pid, wid, "main",
        model=resolve_model(ctx, MODELS_BY_PROVIDER),
        layer=0, timeout=5, prompt=PROMPT,
        validation_commands=["true"],
    )
    wfi = ctx.client.run_project_workflow(
        pid, wid, instructions="validation pass",
    )["instance_id"]
    wait_for_workflow(ctx, pid, instance_id=wfi)

    sess = first_session(db_mod.agent_sessions_for_instance(ctx.server.home, wfi))
    if sess["status"] not in PASS_STATUSES or sess["result"] != "pass":
        return ("S39 validation_commands pass", "FAIL",
                f"status/result = {sess['status']}/{sess['result']}, "
                f"reason={sess.get('result_reason')!r}")
    if (sess.get("findings") or {}).get("validation_failure") is not None:
        return ("S39 validation_commands pass", "FAIL",
                "unexpected validation_failure finding on pass path")
    msgs = db_mod.agent_messages(ctx.server.home, sess["id"])
    val_rows = [m for m in msgs if m.get("category") == "validation"]
    if not val_rows:
        return ("S39 validation_commands pass", "FAIL",
                "no category='validation' agent_messages rows")
    return ("S39 validation_commands pass", "PASS",
            f"session={sess['id']} validation_rows={len(val_rows)}")
