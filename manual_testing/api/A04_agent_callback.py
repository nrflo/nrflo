"""A04 — api-mode `agent_callback` terminal signal triggers L0 replay.

Mirrors engine/s17 but exercises the apirun callback path: the L1 agent
calls `agent_callback` with `{"level": 0}`, the handler emits
`TerminalSignal{Status:"CALLBACK", Level:0}`, `finalizePhase` reads
the level and re-spawns L0. Stops after the second L0 row appears
because the naive prompt would loop forever.

Expected PASS:
  - >= 2 agent_sessions rows with agent_type='a' (the L0 replay).
  - First agent_type='b' session has result == 'callback'.
"""

from __future__ import annotations

import time

from lib import db as db_mod
from lib.runtime import Ctx, Result, make_project, next_id, resolve_model


MODELS_BY_PROVIDER: dict[str, str] = {}

L0_PROMPT = """\
You are an integration-test agent in api-mode. Call these tools in order:
  1. `findings_add` with {"key": "greet", "value": "hi"}
  2. `agent_finished` with {}
Then stop.
"""

L1_PROMPT = """\
You are the L1 verifier in an api-mode layered integration test. The
L0 agent produced a single finding `greet`. The test ONLY passes if
you call exactly one tool:

  `agent_callback` with input {"level": 0}

Do NOT call `agent_finished`. Do NOT emit any other text or tool call.
Invoke `agent_callback` as your first and only tool use.
"""


def run(ctx: Ctx) -> Result:
    pid, _root = make_project(ctx)
    wid = next_id(ctx, "wf")
    ctx.client.create_workflow(pid, wid, scope_type="project")
    ctx.client.create_agent_def(
        pid, wid, "a",
        model=resolve_model(ctx, MODELS_BY_PROVIDER),
        layer=0, timeout=60, prompt=L0_PROMPT,
        tools="findings_add,agent_finished",
    )
    ctx.client.create_agent_def(
        pid, wid, "b",
        model=resolve_model(ctx, MODELS_BY_PROVIDER),
        layer=1, timeout=60, prompt=L1_PROMPT,
        tools="agent_callback",
    )
    wfi = ctx.client.run_project_workflow(
        pid, wid, instructions="api-mode callback",
    )["instance_id"]

    deadline = time.monotonic() + 180.0
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
        return ("A04 agent callback", "FAIL",
                f"L0 did not re-run (a_count={a_count})")
    first_b = next((s for s in sessions if s["agent_type"] == "b"), None)
    if not first_b or first_b.get("result") != "callback":
        return ("A04 agent callback", "FAIL",
                f"first b session result="
                f"{first_b.get('result') if first_b else None}")
    return ("A04 agent callback", "PASS",
            f"callback triggered L0 replay (a_count={a_count})")
