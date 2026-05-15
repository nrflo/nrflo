"""S23 — Workflow chain handoff via `chain-next-ticket`.

Tests:
  - A 2-step chain where step 1 is ticket-scope with require_ticket_handoff.
  - Step 0's agent calls `nrflo agent chain-next-ticket --ticket-id <id>`
    to nominate the ticket; the chain runner must materialise step 1
    against that ticket.

Expected PASS result:
  - workflow_chain_run.status == 'completed'.
  - Step 1's ticket_id matches the nominated ticket.
  - Step 1's workflow_instance.scope_type == 'ticket'.
"""

from __future__ import annotations

import time

from lib import db as db_mod
from lib.runtime import Ctx, Result, make_project, next_id, resolve_model


# Per-provider model overrides; empty = use the runner default (e.g. haiku).
MODELS_BY_PROVIDER: dict[str, str] = {}


A_PROMPT_TMPL = """\
You are an integration-test agent. Do EXACTLY what is listed below and
nothing else. Use the Bash tool to run the listed commands in order,
then stop immediately.

1. Run: `nrflo agent chain-next-ticket --ticket-id {ticket_id}`
2. Run: `nrflo agent finished`
"""

B_PROMPT = """\
You are an integration-test agent. Do EXACTLY what is listed below and
nothing else. Use the Bash tool to run the listed command, then stop.

1. Run: `nrflo agent finished`
"""


def run(ctx: Ctx) -> Result:
    pid, _root = make_project(ctx)

    # Pre-create the ticket that step 1 will run against.
    tid = next_id(ctx, "tk")
    ctx.client.create_ticket(pid, ticket_id=tid, title="chain handoff target")

    wa = next_id(ctx, "wf")
    ctx.client.create_workflow(pid, wa, scope_type="project")
    ctx.client.create_agent_def(
        pid, wa, "main",
        model=resolve_model(ctx, MODELS_BY_PROVIDER), layer=0, timeout=5,
        prompt=A_PROMPT_TMPL.format(ticket_id=tid),
    )

    wb = next_id(ctx, "wf")
    ctx.client.create_workflow(pid, wb, scope_type="ticket",
                                close_ticket_on_complete=False)
    ctx.client.create_agent_def(
        pid, wb, "main",
        model=resolve_model(ctx, MODELS_BY_PROVIDER), layer=0, timeout=5,
        prompt=B_PROMPT,
    )

    chain_id = next_id(ctx, "chain")
    ctx.client.create_workflow_chain(pid, chain_id, steps=[
        {"workflow_name": wa, "scope_type": "project"},
        {"workflow_name": wb, "scope_type": "ticket",
         "require_ticket_handoff": True},
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
        time.sleep(1)
    if detail.get("status") != "completed":
        return ("S23 chain-next-ticket", "FAIL",
                f"chain status={detail.get('status')!r}")
    db_steps = db_mod.chain_run_steps(ctx.server.home, run_id)
    if len(db_steps) != 2:
        return ("S23 chain-next-ticket", "FAIL",
                f"step count={len(db_steps)}")
    if db_steps[1].get("ticket_id") != tid:
        return ("S23 chain-next-ticket", "FAIL",
                f"step 1 ticket_id={db_steps[1].get('ticket_id')!r}, "
                f"want {tid!r}")
    inst_b = db_mod.workflow_instance(
        ctx.server.home, db_steps[1].get("workflow_instance_id"))
    if not inst_b or inst_b.get("scope_type") != "ticket":
        return ("S23 chain-next-ticket", "FAIL",
                f"step 1 wfi scope_type={(inst_b or {}).get('scope_type')!r}")
    return ("S23 chain-next-ticket", "PASS",
            f"chain handed off to ticket {tid}")
