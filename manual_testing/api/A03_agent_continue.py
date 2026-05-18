"""A03 — api-mode `agent_continue` terminal signal triggers relaunch.

Verifies that `TerminalSignal{Status:"CONTINUE"}` reaches
`relaunchForContinuation` and produces a fresh agent_session row with
carried-over findings.

Expected PASS:
  - 2 agent_sessions rows for the wfi.
  - sessions[0].result == 'continue', result_reason == 'api_continue'.
  - sessions[1].findings.gate == 'set' (carried over from session[0]).
"""

from __future__ import annotations

from lib import db as db_mod
from lib.runtime import (
    Ctx, Result, make_project, next_id, resolve_model, wait_for_workflow,
)


MODELS_BY_PROVIDER: dict[str, str] = {}

PROMPT = """\
You are an integration-test agent running in api-mode.

First, call `findings_get` with input {"key": "gate"} and read the
result.

- If `gate` is NOT set (the result indicates the key is missing), call
  the tools in this exact order, each as its own tool call:
    1. `findings_add` with {"key": "gate", "value": "set"}
    2. `agent_continue` with {}
  Then stop.

- If `gate` IS set to "set", you are the RELAUNCHED attempt. Call:
    1. `agent_finished` with {}
  Then stop. Do not write any other findings.
"""


def run(ctx: Ctx) -> Result:
    pid, _root = make_project(ctx)
    wid = next_id(ctx, "wf")
    ctx.client.create_workflow(pid, wid, scope_type="project")
    ctx.client.create_agent_def(
        pid, wid, "main",
        model=resolve_model(ctx, MODELS_BY_PROVIDER),
        layer=0, timeout=120, prompt=PROMPT,
        tools="findings_add,findings_get,agent_continue,agent_finished",
    )
    wfi = ctx.client.run_project_workflow(
        pid, wid, instructions="api-mode continue",
    )["instance_id"]
    wait_for_workflow(ctx, pid, instance_id=wfi)

    sessions = db_mod.agent_sessions_for_instance(ctx.server.home, wfi)
    if len(sessions) < 2:
        return ("A03 agent continue", "FAIL",
                f"session count = {len(sessions)}, want >= 2 (initial + continue)")

    first, second = sessions[0], sessions[1]
    if first.get("result") != "continue":
        return ("A03 agent continue", "FAIL",
                f"session[0].result = {first.get('result')!r}, want 'continue'")
    if first.get("result_reason") != "api_continue":
        return ("A03 agent continue", "FAIL",
                f"session[0].result_reason = {first.get('result_reason')!r}, "
                "want 'api_continue'")
    if (first.get("findings") or {}).get("gate") != "set":
        return ("A03 agent continue", "FAIL",
                f"session[0].findings.gate missing/wrong "
                f"(got {(first.get('findings') or {}).get('gate')!r})")
    if (second.get("findings") or {}).get("gate") != "set":
        return ("A03 agent continue", "FAIL",
                f"session[1].findings.gate not carried over "
                f"(got {(second.get('findings') or {}).get('gate')!r})")
    return ("A03 agent continue", "PASS",
            f"continued (sessions={len(sessions)}, second={second['id']})")
