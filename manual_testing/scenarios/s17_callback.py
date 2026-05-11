"""S17 — Layer callback re-spawns earlier layer.

Tests:
  - L1 calls `nrflo agent callback --level 0`, which marks L1's session
    result='callback' and triggers the orchestrator to re-spawn L0.
  - The naive prompt would loop forever (L1 always calls back), so we
    observe the first L0 replay and then force-stop.

Expected PASS result:
  - ≥ 2 L0 (agent_type='a') sessions exist for the wfi (the replay).
  - First L1 (agent_type='b') session has result='callback'.
"""

from __future__ import annotations

import time

from lib import db as db_mod
from lib.runtime import Ctx, Result, make_project, next_id, resolve_model


# Per-provider model overrides; empty = use the runner default (e.g. haiku).
MODELS_BY_PROVIDER: dict[str, str] = {}


L0_PROMPT = """\
You are an integration-test agent. Do EXACTLY what is listed below and
nothing else. Use the Bash tool to run the listed commands in order,
then stop immediately.

1. Run: `nrflo findings add greet hi`
2. Run: `nrflo agent finished`
"""

L1_PROMPT = """\
You are an integration-test agent. Do EXACTLY what is listed below and
nothing else. Run the command via the Bash tool, then stop.

1. Run: `nrflo agent callback --level 0`
"""


def run(ctx: Ctx) -> Result:
    pid, _root = make_project(ctx)
    wid = next_id(ctx, "wf")
    ctx.client.create_workflow(pid, wid, scope_type="project")
    ctx.client.create_agent_def(
        pid, wid, "a", model=resolve_model(ctx, MODELS_BY_PROVIDER), layer=0, timeout=5, prompt=L0_PROMPT)
    ctx.client.create_agent_def(
        pid, wid, "b", model=resolve_model(ctx, MODELS_BY_PROVIDER), layer=1, timeout=5, prompt=L1_PROMPT)
    wfi = ctx.client.run_project_workflow(
        pid, wid, instructions="callback test",
    )["instance_id"]

    deadline = time.monotonic() + 90.0
    while time.monotonic() < deadline:
        sessions = db_mod.agent_sessions_for_instance(ctx.server.home, wfi)
        if sum(1 for s in sessions if s["agent_type"] == "a") >= 2:
            break
        time.sleep(2)
    try:
        ctx.client.stop_project_workflow(pid, instance_id=wfi)
    except Exception:
        pass

    sessions = db_mod.agent_sessions_for_instance(ctx.server.home, wfi)
    a_count = sum(1 for s in sessions if s["agent_type"] == "a")
    if a_count < 2:
        return ("S17 callback", "FAIL", f"L0 did not re-run (a_count={a_count})")
    first_b = next((s for s in sessions if s["agent_type"] == "b"), None)
    if not first_b or first_b.get("result") != "callback":
        return ("S17 callback", "FAIL",
                f"first b session result="
                f"{first_b.get('result') if first_b else None}")
    return ("S17 callback", "PASS",
            f"callback triggered L0 replay (a_count={a_count})")
