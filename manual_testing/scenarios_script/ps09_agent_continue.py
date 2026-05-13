"""PS09 — script: agent.continue_() — relaunch path for script mode.

Tests SDK method: `c.agent.continue_()`.

Script-mode is exempt from context-save (TracksContext=false in
`backend_script.go`). The continue call is still routed through the
socket and tagged on the session row. This scenario verifies the SDK
call succeeds end-to-end and the session result is recorded.

Asserts loosely: result ∈ {"continue", "pass"} — both are acceptable
outcomes depending on the relaunch path chosen by the orchestrator for
non-context-tracking backends. The strict assertion is that the SDK
call itself does NOT error (any exception would propagate as a
non-zero exit and we'd see result=fail).
"""

from __future__ import annotations

from lib import db as db_mod
from lib.runtime import (
    Ctx, Result, make_project, next_id, wait_for_workflow,
)
from lib.script_helpers import make_script_agent


# Use a workflow-instance-scoped sentinel so the relaunch sees it and
# returns immediately; unique per test run so /tmp pollution across
# invocations can't accidentally short-circuit the first call.
CODE = """
import os, pathlib
sentinel = pathlib.Path("/tmp") / (os.environ["NRF_WORKFLOW_INSTANCE_ID"] + "_ps09.flag")
if sentinel.exists():
    c.findings.add("relaunched", "yes")
    c.agent.finished()
else:
    sentinel.write_text("1")
    c.findings.add("first_run", "yes")
    c.agent.continue_()
"""


def run(ctx: Ctx) -> Result:
    pid, _root = make_project(ctx)
    wid = next_id(ctx, "wf")
    ctx.client.create_workflow(pid, wid, scope_type="project")
    make_script_agent(ctx, pid, wid, "main", code=CODE, max_fail_restarts=1)
    wfi = ctx.client.run_project_workflow(
        pid, wid, instructions="ps09",
    )["instance_id"]
    try:
        wait_for_workflow(ctx, pid, instance_id=wfi)
    except TimeoutError:
        try:
            ctx.client.stop_project_workflow(pid, instance_id=wfi)
        except Exception:
            pass

    sessions = db_mod.agent_sessions_for_instance(ctx.server.home, wfi)
    if not sessions:
        return ("PS09 agent.continue", "FAIL", "no sessions recorded")
    first = sessions[0]
    if first.get("result") not in ("continue", "pass"):
        return ("PS09 agent.continue", "FAIL",
                f"first.result = {first.get('result')!r} (expected continue/pass)")
    # The first session MUST have recorded the SDK call (no error path).
    if (first.get("findings") or {}).get("first_run") != "yes":
        return ("PS09 agent.continue", "FAIL",
                f"first.findings = {first.get('findings')!r}")
    return ("PS09 agent.continue", "PASS",
            f"continue call succeeded, sessions={len(sessions)}, "
            f"first.result={first.get('result')}")
