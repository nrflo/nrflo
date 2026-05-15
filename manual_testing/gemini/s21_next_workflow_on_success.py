"""S21 — Auto-chain via next_workflow_on_success.

Tests:
  - WF_A declares `next_workflow_on_success=WF_B`.
  - WF_A's agent writes a `workflow_final_result` finding.
  - After WF_A completes successfully, the orchestrator auto-spawns
    WF_B with instructions=<WF_A's workflow_final_result>.

Expected PASS result:
  - A workflow_instances row exists for WF_B within ~60s of WF_A's end.
  - WF_B's agent_sessions.prompt contains 'forwarded value'.
"""

from __future__ import annotations

import time

from lib import db as db_mod
from lib.runtime import (
    Ctx, Result, first_session, make_project, next_id, resolve_model, wait_for_workflow,
)


# Per-provider model overrides; empty = use the runner default (e.g. haiku).
MODELS_BY_PROVIDER: dict[str, str] = {}


A_PROMPT = """\
You are an integration-test agent. Do EXACTLY what is listed below and
nothing else. Use the Bash tool to run the listed commands in order,
then stop immediately.

1. Run: `nrflo findings add workflow_final_result "forwarded value"`
2. Run: `nrflo agent finished`
"""

B_PROMPT = """\
You are an integration-test agent. Do EXACTLY what is listed below and
nothing else. Use the Bash tool to run the listed command, then stop.

1. Run: `nrflo agent finished`
"""


def run(ctx: Ctx) -> Result:
    pid, _root = make_project(ctx)

    wb = next_id(ctx, "wf")
    ctx.client.create_workflow(pid, wb, scope_type="project")
    ctx.client.create_agent_def(
        pid, wb, "main",
        model=resolve_model(ctx, MODELS_BY_PROVIDER), layer=0, timeout=5, prompt=B_PROMPT,
    )
    wa = next_id(ctx, "wf")
    ctx.client.create_workflow(
        pid, wa, scope_type="project", next_workflow_on_success=wb)
    ctx.client.create_agent_def(
        pid, wa, "main",
        model=resolve_model(ctx, MODELS_BY_PROVIDER), layer=0, timeout=5, prompt=A_PROMPT,
    )
    wfi_a = ctx.client.run_project_workflow(
        pid, wa, instructions="autochain",
    )["instance_id"]
    wait_for_workflow(ctx, pid, instance_id=wfi_a)

    deadline = time.monotonic() + 60.0
    wfi_b: str | None = None
    while time.monotonic() < deadline:
        state = ctx.client.get_project_workflow_state(pid, workflow_id=wb)
        for iid, wf in (state.get("all_workflows") or {}).items():
            if (wf or {}).get("workflow") == wb:
                wfi_b = iid
                break
        if wfi_b:
            break
        time.sleep(2)
    if not wfi_b:
        return ("S21 next_workflow_on_success", "FAIL",
                "WF_B never auto-spawned")
    wait_for_workflow(ctx, pid, instance_id=wfi_b)

    sess = first_session(
        db_mod.agent_sessions_for_instance(ctx.server.home, wfi_b))
    if "forwarded value" not in (sess.get("prompt") or ""):
        return ("S21 next_workflow_on_success", "FAIL",
                "WF_B prompt missing WF_A's workflow_final_result")
    return ("S21 next_workflow_on_success", "PASS",
            "auto-chained with forwarded final result")
