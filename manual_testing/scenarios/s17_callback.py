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

# Models under provider rate-limit pressure sometimes "optimise" a
# one-line prompt by skipping the tool call entirely and just emitting
# "OK, done." This prompt is deliberately written as a contract — the
# model is told the test is asserting on the callback row and given a
# brief context-setting step to keep it engaged. Empirically this is
# the smallest L1 prompt that all three providers follow reliably under
# parallel=5 load (claude haiku, codex gpt-mini, opencode gpt54-mini).
L1_PROMPT = """\
You are the L1 verifier agent in a layered integration test. The L0
agent produced a single finding called `greet` and you must send the
work back to L0 by calling the callback CLI.

The grader of this test is a Go program that asserts:
  * your session's result column is exactly `callback`
  * L0 was re-spawned a second time

Both assertions ONLY pass if you invoke `nrflo agent callback --level 0`
as your tool call. Do NOT call `nrflo agent finished`. Do NOT output a
plain-text summary in place of the tool call. The callback CLI is the
single, contractual action this layer performs.

Step 1: Use the Bash tool to read what L0 left for you:
    `nrflo findings get greet`
    Acknowledge in one sentence what you saw.

Step 2: Use the Bash tool to issue the callback:
    `nrflo agent callback --level 0`

Stop after step 2. Do not run any other command.
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
