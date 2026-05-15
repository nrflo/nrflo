"""S30 — Ticket concurrency guard returns 409 on a running workflow.

Tests:
  - `POST /api/v1/tickets/{id}/workflow/run` with an already-running
    workflow on the same (project, ticket, workflow) triplet must
    return HTTP 409 — regardless of the `force` body flag (the
    concurrency check in `handlers_orchestrate.go:84` calls
    `orchestrator.IsRunning` unconditionally; `force` only relaxes
    the worktree guard for non-project scopes, see
    `orchestrator.go:207`).

Expected PASS:
  - 1st run → 200.
  - 2nd run while 1st is still running, force=false → 409.
  - 3rd run while 1st is still running, force=true → still 409.
"""

from __future__ import annotations

import time

from lib import db as db_mod
from lib.runtime import (
    Ctx, Result, make_project, next_id, resolve_model,
)


MODELS_BY_PROVIDER: dict[str, str] = {}

# Long enough that we have time to fire the 2nd + 3rd POSTs while 1st runs.
PROMPT = """\
You are an integration-test agent. Use the Bash tool to run the listed
commands in order, then stop.

1. Run: `sleep 30`
2. Run: `nrflo agent finished`
"""


def _wait_for_running(ctx: Ctx, wfi: str, timeout_s: float = 60.0) -> bool:
    deadline = time.monotonic() + timeout_s
    while time.monotonic() < deadline:
        sessions = db_mod.agent_sessions_for_instance(ctx.server.home, wfi)
        if sessions and sessions[0].get("status") == "running":
            return True
        time.sleep(0.5)
    return False


def run(ctx: Ctx) -> Result:
    pid, _root = make_project(ctx)
    wid = next_id(ctx, "wf")
    tid = next_id(ctx, "t")
    ctx.client.create_workflow(
        pid, wid, scope_type="ticket", close_ticket_on_complete=False,
    )
    ctx.client.create_agent_def(
        pid, wid, "main",
        model=resolve_model(ctx, MODELS_BY_PROVIDER),
        layer=0, timeout=5, prompt=PROMPT,
    )
    ctx.client.create_ticket(pid, ticket_id=tid, title="s30")

    run1 = ctx.client.run_ticket_workflow(
        pid, tid, workflow_id=wid, instructions="run-1",
    )
    wfi1 = run1["instance_id"]

    # Wait until the agent is actually running before testing the guard;
    # the guard checks `orchestrator.IsRunning(...)`.
    if not _wait_for_running(ctx, wfi1):
        try:
            ctx.client.stop_project_workflow(pid, instance_id=wfi1)
        except Exception:
            pass
        return ("S30 concurrency guard", "FAIL",
                "first run never reached status=running")

    # 2nd POST: no force → expect 409.
    status2, body2 = ctx.client.run_ticket_workflow(
        pid, tid, workflow_id=wid, instructions="run-2",
        force=False, expect_status=409,
    )
    if status2 != 409:
        try:
            ctx.client.stop_project_workflow(pid, instance_id=wfi1)
        except Exception:
            pass
        return ("S30 concurrency guard", "FAIL",
                f"2nd run (force=false) returned {status2}, want 409 "
                f"(body={body2!r})")

    # 3rd POST: force=true while still running — guard still trips.
    status3, body3 = ctx.client.run_ticket_workflow(
        pid, tid, workflow_id=wid, instructions="run-3", force=True,
        expect_status=409,
    )

    try:
        ctx.client.stop_project_workflow(pid, instance_id=wfi1)
    except Exception:
        pass

    if status3 != 409:
        return ("S30 concurrency guard", "FAIL",
                f"3rd run (force=true) returned {status3}, want 409 — "
                f"force should NOT bypass concurrency (body={body3!r})")

    return ("S30 concurrency guard", "PASS",
            f"409 on force=false and force=true while wfi {wfi1[:8]} runs")
