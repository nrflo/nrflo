"""S34 — Multiple workflow_instances rows for the same (ticket, workflow).

Tests:
  - Migration 000040 dropped the UNIQUE index on (ticket_id, workflow_id).
    Running the same ticket-scoped workflow twice sequentially must
    create two rows, both linked to the same ticket.

Expected PASS:
  - ≥2 workflow_instances rows exist with this (project_id, workflow_id)
    and the same ticket_id; both reach a terminal state.
"""

from __future__ import annotations

from lib import db as db_mod
from lib.runtime import (
    Ctx, Result, make_project, next_id, resolve_model, wait_for_workflow,
)


MODELS_BY_PROVIDER: dict[str, str] = {}


PROMPT = """\
You are an integration-test agent. Use the Bash tool to run the listed
command, then stop.

1. Run: `nrflo agent finished`
"""


def run(ctx: Ctx) -> Result:
    pid, _root = make_project(ctx)
    wid = next_id(ctx, "wf")
    tid = next_id(ctx, "t")
    # close_ticket_on_complete=False so the second run isn't fighting a
    # closed-ticket reopen race.
    ctx.client.create_workflow(
        pid, wid, scope_type="ticket", close_ticket_on_complete=False,
    )
    ctx.client.create_agent_def(
        pid, wid, "main",
        model=resolve_model(ctx, MODELS_BY_PROVIDER),
        layer=0, timeout=5, prompt=PROMPT,
    )
    ctx.client.create_ticket(pid, ticket_id=tid, title="s34")

    run1 = ctx.client.run_ticket_workflow(pid, tid, workflow_id=wid)
    wfi1 = run1["instance_id"]
    wait_for_workflow(ctx, pid, instance_id=wfi1, ticket_id=tid)

    run2 = ctx.client.run_ticket_workflow(pid, tid, workflow_id=wid)
    wfi2 = run2["instance_id"]
    wait_for_workflow(ctx, pid, instance_id=wfi2, ticket_id=tid)

    if wfi1 == wfi2:
        return ("S34 multi-instance same ticket", "FAIL",
                f"second run returned the same instance_id: {wfi1}")

    rows = db_mod.workflow_instances_for_workflow(ctx.server.home, pid, wid)
    if len(rows) < 2:
        return ("S34 multi-instance same ticket", "FAIL",
                f"workflow_instances count = {len(rows)}, want >= 2")

    inst1 = db_mod.workflow_instance(ctx.server.home, wfi1)
    inst2 = db_mod.workflow_instance(ctx.server.home, wfi2)
    if not inst1 or not inst2:
        return ("S34 multi-instance same ticket", "FAIL",
                f"missing instance rows: inst1={bool(inst1)} inst2={bool(inst2)}")
    if inst1.get("ticket_id") != tid or inst2.get("ticket_id") != tid:
        return ("S34 multi-instance same ticket", "FAIL",
                f"ticket_id mismatch: {inst1.get('ticket_id')}, "
                f"{inst2.get('ticket_id')}, want {tid}")

    return ("S34 multi-instance same ticket", "PASS",
            f"instances={len(rows)} ticket={tid}")
