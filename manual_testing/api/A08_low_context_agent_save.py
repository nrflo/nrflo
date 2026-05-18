"""A08 — api-mode trips low-context + relaunches (forced agent-save).

`shouldUseAgentSave` in `be/internal/spawner/context_save.go` returns
true unconditionally for the api backend, so the low-context path
spawns the `context-saver-api` system agent rather than `--resume`.
With `restart_threshold=100` the spawner trips low-context on the
first reported `context_left`, and `relaunchForContinuation` produces
a second main session.

We do NOT assert on the `to_resume` finding because migration 63 docs
the api-mode saver's cross-session write as a follow-up concern:
`findings_add` always writes to the saver's own session, so the
carry-over path that exists for CLI agents (saver → main session) does
not yet apply to api-mode. The unit-level write path is covered in
`be/internal/spawner/findings_carryover_test.go`.

Expected PASS:
  - >= 2 agent_sessions rows with agent_type='main' for the wfi.
  - main[0].result_reason == 'low_context' (proves the trip + saver
    flow + relaunch path).
"""

from __future__ import annotations

import time

from lib import db as db_mod
from lib.runtime import (
    Ctx, Result, make_project, next_id, resolve_model,
)


MODELS_BY_PROVIDER: dict[str, str] = {}

MAIN_AGENT = "main"
POLL_INTERVAL_S = 0.5
DETECT_TIMEOUT_S = 300.0


PROMPT = """\
You are an integration-test agent in api-mode.

First, call `findings_get` with input {"keys": ["to_resume"]}.

- If the returned value for `to_resume` is empty, this is the FIRST
  attempt. Call these tools in order, each as its own turn:
    1. `findings_add` with {"key": "step1", "value": "done"}
    2. `findings_add` with {"key": "step2", "value": "done"}
    3. `findings_add` with {"key": "step3", "value": "done"}
    4. `findings_add` with {"key": "step4", "value": "done"}
    5. `agent_finished` with {}
  Then stop.

- If `to_resume` is set to a non-empty string, this is the RELAUNCHED
  attempt. Call:
    1. `agent_finished` with {}
  Then stop.
"""


def _main_sessions(ctx: Ctx, wfi: str) -> list[dict]:
    return [
        s for s in db_mod.agent_sessions_for_instance(ctx.server.home, wfi)
        if s.get("agent_type") == MAIN_AGENT
    ]


def run(ctx: Ctx) -> Result:
    pid, _root = make_project(ctx)
    wid = next_id(ctx, "wf")
    ctx.client.create_workflow(pid, wid, scope_type="project")
    ctx.client.create_agent_def(
        pid, wid, MAIN_AGENT,
        model=resolve_model(ctx, MODELS_BY_PROVIDER),
        layer=0, timeout=120, prompt=PROMPT,
        tools="findings_add,findings_get,agent_finished",
        restart_threshold=100,
    )
    wfi = ctx.client.run_project_workflow(
        pid, wid, instructions="api-mode low context",
    )["instance_id"]

    deadline = time.monotonic() + DETECT_TIMEOUT_S
    sessions: list[dict] = []
    while time.monotonic() < deadline:
        sessions = _main_sessions(ctx, wfi)
        if len(sessions) >= 2:
            break
        time.sleep(POLL_INTERVAL_S)

    try:
        ctx.client.stop_project_workflow(pid, instance_id=wfi)
    except Exception:
        pass

    time.sleep(1.0)
    sessions = _main_sessions(ctx, wfi)
    if len(sessions) < 2:
        return ("A08 low_context agent_save", "FAIL",
                f"main session count = {len(sessions)}, want >= 2")

    first = sessions[0]
    if first.get("result_reason") != "low_context":
        return ("A08 low_context agent_save", "FAIL",
                f"main[0].result_reason = {first.get('result_reason')!r}, "
                "want 'low_context'")
    return ("A08 low_context agent_save", "PASS",
            f"low_context trip + relaunch ok (main_sessions={len(sessions)})")
