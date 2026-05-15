"""PS18 — script: c.agent.chain_next_ticket + require_ticket_handoff.

Tests:
  - The Python SDK's `c.agent.chain_next_ticket(ticket_id)` method
    forwards to the `agent.chain_next_ticket` socket method
    (`be/internal/socket/handler.go`), same code path the CLI's
    `nrflo agent chain-next-ticket` uses.
  - A two-step chain (step 0 project-scope → step 1 ticket-scope with
    `require_ticket_handoff=true`) runs step 1 against the ticket the
    SDK call nominated.

Mirrors s23 (CLI shape) but pinned to the script backend so the test
doesn't depend on LLM instruction-following.
"""

from __future__ import annotations

import time

from lib import db as db_mod
from lib.runtime import (
    Ctx, Result, make_project, next_id,
)
from lib.script_helpers import make_script_agent


STEP_A_CODE_TMPL = """
c.agent.chain_next_ticket({tid!r})
c.agent.finished()
"""

STEP_B_CODE = """
c.agent.finished()
"""


def run(ctx: Ctx) -> Result:
    pid, _root = make_project(ctx)

    tid = next_id(ctx, "tk")
    ctx.client.create_ticket(pid, ticket_id=tid, title="chain handoff target")

    wa = next_id(ctx, "wf")
    ctx.client.create_workflow(pid, wa, scope_type="project")
    make_script_agent(ctx, pid, wa, "main",
                      code=STEP_A_CODE_TMPL.format(tid=tid))

    wb = next_id(ctx, "wf")
    ctx.client.create_workflow(pid, wb, scope_type="ticket",
                               close_ticket_on_complete=False)
    make_script_agent(ctx, pid, wb, "main", code=STEP_B_CODE)

    chain_id = next_id(ctx, "chain")
    ctx.client.create_workflow_chain(pid, chain_id, steps=[
        {"workflow_name": wa, "scope_type": "project"},
        {"workflow_name": wb, "scope_type": "ticket",
         "require_ticket_handoff": True},
    ])
    run_resp = ctx.client.start_workflow_chain_run(
        pid, chain_id, instructions="ps18", triggered_by="manual",
    )
    run_id = run_resp["id"]

    deadline = time.monotonic() + 60.0
    detail: dict = {}
    while time.monotonic() < deadline:
        detail = ctx.client.get_workflow_chain_run(pid, chain_id, run_id)
        if detail.get("status") in ("completed", "failed", "canceled"):
            break
        time.sleep(0.5)

    if detail.get("status") != "completed":
        return ("PS18 chain_next_ticket (SDK)", "FAIL",
                f"chain status={detail.get('status')!r}")

    steps = db_mod.chain_run_steps(ctx.server.home, run_id)
    if len(steps) != 2:
        return ("PS18 chain_next_ticket (SDK)", "FAIL",
                f"step count={len(steps)}")
    if steps[1].get("ticket_id") != tid:
        return ("PS18 chain_next_ticket (SDK)", "FAIL",
                f"step 1 ticket_id={steps[1].get('ticket_id')!r}, want {tid!r}")
    inst_b = db_mod.workflow_instance(
        ctx.server.home, steps[1].get("workflow_instance_id"))
    if not inst_b or inst_b.get("scope_type") != "ticket":
        return ("PS18 chain_next_ticket (SDK)", "FAIL",
                f"step 1 wfi scope_type={(inst_b or {}).get('scope_type')!r}")
    return ("PS18 chain_next_ticket (SDK)", "PASS",
            f"chain handed off to {tid}")
