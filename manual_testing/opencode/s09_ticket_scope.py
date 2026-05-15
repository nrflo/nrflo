"""S09 — Ticket-scope happy path with auto-close.

Tests:
  - A ticket-scope workflow whose only agent runs `nrflo agent finished`.
  - When workflow.close_ticket_on_complete=true, the ticket auto-closes
    after successful completion.

Expected PASS result:
  - workflow_instances.scope_type == 'ticket'
  - workflow_instances.status ∈ {completed, project_completed}
  - tickets.status == 'closed'
"""

from __future__ import annotations

from lib import db as db_mod
from lib.runtime import (
    Ctx, PASS_STATUSES, Result, make_project, next_id, resolve_model, wait_for_workflow,
)


# Per-provider model overrides; empty = use the runner default (e.g. haiku).
MODELS_BY_PROVIDER: dict[str, str] = {}


PROMPT = """\
You are an integration-test agent. Do EXACTLY what is listed below and
nothing else. Use the Bash tool to run the listed command, then stop.

1. Run: `nrflo agent finished`
"""


def run(ctx: Ctx) -> Result:
    pid, _root = make_project(ctx)
    wid = next_id(ctx, "wf")
    ctx.client.create_workflow(
        pid, wid, scope_type="ticket", close_ticket_on_complete=True)
    ctx.client.create_agent_def(
        pid, wid, "main",
        model=resolve_model(ctx, MODELS_BY_PROVIDER), layer=0, timeout=5, prompt=PROMPT,
    )
    tid = next_id(ctx, "tk")
    ctx.client.create_ticket(pid, ticket_id=tid, title="manual test ticket")
    wfi = ctx.client.run_ticket_workflow(
        pid, tid, workflow_id=wid,
    )["instance_id"]
    wait_for_workflow(ctx, pid, instance_id=wfi, ticket_id=tid)

    inst = db_mod.workflow_instance(ctx.server.home, wfi)
    if not inst or inst.get("scope_type") != "ticket":
        return ("S09 ticket scope", "FAIL", f"scope_type = {inst}")
    if inst.get("status") not in PASS_STATUSES:
        return ("S09 ticket scope", "FAIL",
                f"wfi status = {inst.get('status')}")
    tk = db_mod.ticket(ctx.server.home, pid, tid)
    if not tk or tk.get("status") != "closed":
        return ("S09 ticket scope", "FAIL", f"ticket not closed: {tk}")
    return ("S09 ticket scope", "PASS", f"ticket {tid} closed")
