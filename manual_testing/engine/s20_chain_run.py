"""S20 — Workflow chain run with cross-step handoff.

Tests:
  - Create a chain definition with two project-scope steps (WF_A → WF_B).
  - Start a chain run; step A's agent calls
    the `chain_next_instructions` tool to set step B's instructions.
  - Chain run reaches `completed`; both steps reach `completed`;
    step B's rendered prompt contains the handoff text.

Expected PASS result:
  - workflow_chain_run.status == 'completed'.
  - Both steps have status='completed'.
  - Step B agent_sessions.prompt contains the handoff string.
"""

from __future__ import annotations

import time

from lib import db as db_mod
from lib.runtime import Ctx, Result, make_project, next_id, resolve_model


# Per-provider model overrides; empty = use the runner default (e.g. haiku-4-5).
MODELS_BY_PROVIDER: dict[str, str] = {}


HANDOFF = "hello from step 0"

A_PROMPT = """\
You are an integration-test agent. Do EXACTLY what is listed below and
nothing else. Perform the listed steps in order,
then stop immediately.

1. Run: the `chain_next_instructions` tool (instructions "hello from step 0")
2. Run: the `agent_finished` tool
"""

B_PROMPT = """\
You are an integration-test agent. Do EXACTLY what is listed below and
nothing else. Perform the listed step, then stop.

1. Run: the `agent_finished` tool
"""


def run(ctx: Ctx) -> Result:
    pid, _root = make_project(ctx)

    wa = next_id(ctx, "wf")
    ctx.client.create_workflow(pid, wa, scope_type="project")
    ctx.client.create_agent_def(
        pid, wa, "main",
        model=resolve_model(ctx, MODELS_BY_PROVIDER), layer=0, timeout=5, prompt=A_PROMPT,
    )
    wb = next_id(ctx, "wf")
    ctx.client.create_workflow(pid, wb, scope_type="project")
    ctx.client.create_agent_def(
        pid, wb, "main",
        model=resolve_model(ctx, MODELS_BY_PROVIDER), layer=0, timeout=5, prompt=B_PROMPT,
    )
    chain_id = next_id(ctx, "chain")
    ctx.client.create_workflow_chain(pid, chain_id, steps=[
        {"workflow_name": wa, "scope_type": "project"},
        {"workflow_name": wb, "scope_type": "project"},
    ])
    run_resp = ctx.client.start_workflow_chain_run(
        pid, chain_id, instructions="kickoff", triggered_by="manual",
    )
    run_id = run_resp["id"]

    deadline = time.monotonic() + 180.0
    detail: dict = {}
    while time.monotonic() < deadline:
        detail = ctx.client.get_workflow_chain_run(pid, chain_id, run_id)
        if detail.get("status") in ("completed", "failed", "canceled"):
            break
        time.sleep(2)
    if detail.get("status") != "completed":
        return ("S20 chain run", "FAIL", f"chain status={detail.get('status')!r}")
    steps = detail.get("steps") or []
    statuses = [s.get("status") for s in steps]
    if len(steps) != 2 or any(s != "completed" for s in statuses):
        return ("S20 chain run", "FAIL", f"step statuses={statuses}")
    db_steps = db_mod.chain_run_steps(ctx.server.home, run_id)
    if len(db_steps) != 2:
        return ("S20 chain run", "FAIL", f"step count={len(db_steps)}")
    step_b_iid = db_steps[1].get("workflow_instance_id")
    if not step_b_iid:
        return ("S20 chain run", "FAIL",
                "step B workflow_instance_id missing from chain_run_steps")
    b_sessions = db_mod.agent_sessions_for_instance(ctx.server.home, step_b_iid)
    if not b_sessions or HANDOFF not in (b_sessions[0].get("prompt") or ""):
        return ("S20 chain run", "FAIL",
                "step B prompt missing handoff text")
    return ("S20 chain run", "PASS",
            "2-step chain completed with handoff")
