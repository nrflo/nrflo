"""S08 — workflow_final_result surfaces on REST + persists on agent session.

Tests:
  - An agent writes a finding named `workflow_final_result`.
  - The /workflow REST response exposes it as a top-level field
    (computed from agent_sessions.findings, last by ended_at).
  - That same value is present on the last agent_sessions row.

Expected PASS result:
  - REST response top-level `workflow_final_result` == 'all green'
  - agent_sessions.findings.workflow_final_result == 'all green'
"""

from __future__ import annotations

from lib import db as db_mod
from lib.runtime import (
    Ctx, Result, make_project, next_id, resolve_model, pick_instance, wait_for_workflow,
)


# Per-provider model overrides; empty = use the runner default (e.g. haiku).
MODELS_BY_PROVIDER: dict[str, str] = {}


PROMPT = """\
You are an integration-test agent. Do EXACTLY what is listed below and
nothing else. Use the Bash tool to run the listed commands in order,
then stop immediately.

1. Run: `nrflo findings add workflow_final_result "all green"`
2. Run: `nrflo agent finished`
"""


def run(ctx: Ctx) -> Result:
    pid, _root = make_project(ctx)
    wid = next_id(ctx, "wf")
    ctx.client.create_workflow(pid, wid, scope_type="project")
    ctx.client.create_agent_def(
        pid, wid, "main",
        model=resolve_model(ctx, MODELS_BY_PROVIDER), layer=0, timeout=5, prompt=PROMPT,
    )
    wfi = ctx.client.run_project_workflow(
        pid, wid, instructions="final result",
    )["instance_id"]
    state = wait_for_workflow(ctx, pid, instance_id=wfi)

    wf = pick_instance(state, wfi) or {}
    if wf.get("workflow_final_result") != "all green":
        return ("S08 workflow_final_result", "FAIL",
                f"REST = {wf.get('workflow_final_result')!r}")
    sessions = db_mod.agent_sessions_for_instance(ctx.server.home, wfi)
    last = max(sessions, key=lambda s: s.get("ended_at") or "")
    if (last.get("findings") or {}).get("workflow_final_result") != "all green":
        return ("S08 workflow_final_result", "FAIL",
                f"agent_sessions.findings = {last.get('findings')!r}")
    return ("S08 workflow_final_result", "PASS", "REST + DB agree")
