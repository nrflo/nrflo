"""S16 — Spawner stall detection (running-stall path).

Tests:
  - An agent that blocks in `sleep 30` produces no tool/result events
    during the sleep, so with stall_running_timeout_sec=15 the spawner
    must trip its stall detector and mark the session result='continue'
    with reason 'stall_restart_running_stall' (then it auto-relaunches).
  - Early-exits as soon as the first stall is seen — we don't wait for
    final termination because the orchestrator may keep relaunching.

Expected PASS result:
  - At least one agent_sessions row with result_reason containing 'stall'
    within 60 seconds of starting.
"""

from __future__ import annotations

import time

from lib import db as db_mod
from lib.runtime import Ctx, Result, make_project, next_id, resolve_model


# Per-provider model overrides; empty = use the runner default (e.g. haiku).
MODELS_BY_PROVIDER: dict[str, str] = {}


STALL_TIMEOUT_SEC = 15

PROMPT = """\
You are an integration-test agent. Do EXACTLY what is listed below and
nothing else. Use the Bash tool to run the listed commands in order,
then stop immediately.

1. Run: `sleep 30`
2. Run: `nrflo agent finished`
"""


def run(ctx: Ctx) -> Result:
    # cli-interactive is now reachable: ClaudeAdapter.BumpsOnPTYBytes()=false means
    # PTY redraws no longer reset lastMessageTime, so the running-stall timer
    # accumulates normally. codex/opencode cli_interactive skip at harness level.

    pid, _root = make_project(ctx)
    wid = next_id(ctx, "wf")
    ctx.client.create_workflow(pid, wid, scope_type="project")
    ctx.client.create_agent_def(
        pid, wid, "main",
        model=resolve_model(ctx, MODELS_BY_PROVIDER), layer=0, timeout=5, prompt=PROMPT,
        stall_running_timeout_sec=STALL_TIMEOUT_SEC,
        stall_start_timeout_sec=STALL_TIMEOUT_SEC,
    )
    wfi = ctx.client.run_project_workflow(
        pid, wid, instructions="stall test",
    )["instance_id"]

    deadline = time.monotonic() + 60.0
    while time.monotonic() < deadline:
        sessions = db_mod.agent_sessions_for_instance(ctx.server.home, wfi)
        stalled = [s for s in sessions
                   if "stall" in (s.get("result_reason") or "")]
        if stalled:
            try:
                ctx.client.stop_project_workflow(pid, instance_id=wfi)
            except Exception:
                pass
            return ("S16 stall detection", "PASS",
                    f"stall fired: {stalled[0].get('result_reason')!r}")
        time.sleep(2)
    try:
        ctx.client.stop_project_workflow(pid, instance_id=wfi)
    except Exception:
        pass
    return ("S16 stall detection", "FAIL",
            "no stall_* result_reason within 60s")
