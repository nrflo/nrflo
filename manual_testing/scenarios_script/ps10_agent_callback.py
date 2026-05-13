"""PS10 — script: agent.callback(level) re-runs earlier layer.

Tests SDK method: `c.agent.callback(level=0)`.

L1 calls callback(0); orchestrator re-spawns L0. To avoid an infinite
loop, L0 uses a sentinel file to skip its own re-callback path.

Expected PASS:
  - ≥ 2 L0 sessions (agent_type='l0') exist for the wfi (the replay).
  - First L1 (agent_type='l1') session has result='callback'.
"""

from __future__ import annotations

import time

from lib import db as db_mod
from lib.runtime import Ctx, Result, make_project, next_id
from lib.script_helpers import make_script_agent


L0_CODE = """
c.findings.add("layer", "0")
c.agent.finished()
"""

L1_CODE = """
import os, pathlib
# Use workflow instance id (stable across the callback re-spawn but unique
# per test run) so leftover /tmp sentinels from a prior invocation don't
# trick L1 into skipping the callback.
sentinel = pathlib.Path("/tmp") / (os.environ["NRF_WORKFLOW_INSTANCE_ID"] + "_ps10.flag")
if sentinel.exists():
    c.findings.add("second_run", "yes")
    c.agent.finished()
else:
    sentinel.write_text("1")
    c.findings.add("first_run", "yes")
    c.agent.callback(0)
"""


def run(ctx: Ctx) -> Result:
    pid, _root = make_project(ctx)
    wid = next_id(ctx, "wf")
    ctx.client.create_workflow(pid, wid, scope_type="project")
    make_script_agent(ctx, pid, wid, "l0", code=L0_CODE, layer=0)
    make_script_agent(ctx, pid, wid, "l1", code=L1_CODE, layer=1)
    wfi = ctx.client.run_project_workflow(
        pid, wid, instructions="ps10",
    )["instance_id"]

    deadline = time.monotonic() + 60.0
    while time.monotonic() < deadline:
        sessions = db_mod.agent_sessions_for_instance(ctx.server.home, wfi)
        if sum(1 for s in sessions if s["agent_type"] == "l0") >= 2:
            break
        time.sleep(1)
    try:
        ctx.client.stop_project_workflow(pid, instance_id=wfi)
    except Exception:
        pass

    sessions = db_mod.agent_sessions_for_instance(ctx.server.home, wfi)
    l0_count = sum(1 for s in sessions if s["agent_type"] == "l0")
    if l0_count < 2:
        return ("PS10 agent.callback", "FAIL",
                f"L0 did not re-run (l0_count={l0_count})")
    first_l1 = next((s for s in sessions if s["agent_type"] == "l1"), None)
    if not first_l1 or first_l1.get("result") != "callback":
        return ("PS10 agent.callback", "FAIL",
                f"first l1 result = "
                f"{first_l1.get('result') if first_l1 else None}")
    return ("PS10 agent.callback", "PASS",
            f"callback re-spawned L0 (l0_count={l0_count})")
