"""PS14 — script: per-project env var reaches the script process.

The orchestrator loads project env vars at workflow start and forwards
them via `Config.ProjectEnv`; `prepareScriptSpawn` appends them after
nrflo-controlled vars in the script process environment.

Tests:
  - PUT /env-vars/MY_PS_VAR=red propagates to os.environ inside the
    script.
"""

from __future__ import annotations

from lib import db as db_mod
from lib.runtime import (
    Ctx, PASS_STATUSES, Result, first_session,
    make_project, next_id, wait_for_workflow,
)
from lib.script_helpers import make_script_agent


ENV_NAME = "MY_PS_VAR"
ENV_VALUE = "red"

CODE = f"""
import os
c.findings.add("color", os.environ.get({ENV_NAME!r}, "<missing>"))
c.agent.finished()
"""


def run(ctx: Ctx) -> Result:
    pid, _root = make_project(ctx)
    ctx.client.put_project_env_var(pid, ENV_NAME, ENV_VALUE)

    wid = next_id(ctx, "wf")
    ctx.client.create_workflow(pid, wid, scope_type="project")
    make_script_agent(ctx, pid, wid, "main", code=CODE)
    wfi = ctx.client.run_project_workflow(
        pid, wid, instructions="ps14",
    )["instance_id"]
    wait_for_workflow(ctx, pid, instance_id=wfi)

    sess = first_session(
        db_mod.agent_sessions_for_instance(ctx.server.home, wfi))
    if sess["status"] not in PASS_STATUSES or sess["result"] != "pass":
        return ("PS14 project env var", "FAIL",
                f"status/result = {sess['status']}/{sess['result']}")
    color = (sess.get("findings") or {}).get("color")
    if color != ENV_VALUE:
        return ("PS14 project env var", "FAIL",
                f"script saw color={color!r}, want {ENV_VALUE!r}")
    return ("PS14 project env var", "PASS",
            f"{ENV_NAME}={ENV_VALUE} reached script process")
