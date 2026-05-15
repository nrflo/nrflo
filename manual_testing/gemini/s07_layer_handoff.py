"""S07 — Layer handoff via #{FINDINGS:agent:key} template.

Tests:
  - L0 writes a finding via `nrflo findings add`.
  - L1's prompt template references `#{FINDINGS:l0:handoff_value}`; the
    spawner substitutes the literal value when rendering L1's prompt.
  - L1 then writes that resolved value back into its own findings via Bash,
    proving the substitution happened both in the prompt and at exec.

Expected PASS result:
  - L1 agent_sessions.prompt column contains '42'
  - L1 agent_sessions.findings.observed == 42 (or '42')
  - Both sessions result == 'pass'
"""

from __future__ import annotations

from lib import db as db_mod
from lib.runtime import (
    Ctx, Result, make_project, next_id, resolve_model, wait_for_workflow,
)


# Per-provider model overrides; empty = use the runner default (e.g. haiku).
MODELS_BY_PROVIDER: dict[str, str] = {}


L0_PROMPT = """\
You are an integration-test agent. Do EXACTLY what is listed below and
nothing else. Use the Bash tool to run the listed commands in order,
then stop immediately.

1. Run: `nrflo findings add handoff_value 42`
2. Run: `nrflo agent finished`
"""

L1_PROMPT = """\
You are an integration-test agent. Do EXACTLY what is listed below and
nothing else. Use the Bash tool to run the listed commands in order,
then stop immediately.

The previous layer wrote: #{FINDINGS:l0:handoff_value}

1. Run: `nrflo findings add observed #{FINDINGS:l0:handoff_value}`
2. Run: `nrflo agent finished`
"""


def run(ctx: Ctx) -> Result:
    pid, _root = make_project(ctx)
    wid = next_id(ctx, "wf")
    ctx.client.create_workflow(pid, wid, scope_type="project")
    ctx.client.create_agent_def(
        pid, wid, "l0", model=resolve_model(ctx, MODELS_BY_PROVIDER), layer=0, timeout=5, prompt=L0_PROMPT)
    ctx.client.create_agent_def(
        pid, wid, "l1", model=resolve_model(ctx, MODELS_BY_PROVIDER), layer=1, timeout=5, prompt=L1_PROMPT)
    wfi = ctx.client.run_project_workflow(
        pid, wid, instructions="layer handoff",
    )["instance_id"]
    wait_for_workflow(ctx, pid, instance_id=wfi)

    sessions = db_mod.agent_sessions_for_instance(ctx.server.home, wfi)
    by_type = {s["agent_type"]: s for s in sessions}
    if "l0" not in by_type or "l1" not in by_type:
        return ("S07 layer handoff", "FAIL",
                f"missing sessions, got {list(by_type)}")
    if by_type["l1"].get("result") != "pass":
        return ("S07 layer handoff", "FAIL",
                f"l1 result = {by_type['l1'].get('result')!r}")
    if "42" not in (by_type["l1"].get("prompt") or ""):
        return ("S07 layer handoff", "FAIL",
                "l1 prompt did not contain the L0 value '42'")
    observed = (by_type["l1"].get("findings") or {}).get("observed")
    if str(observed) != "42":
        return ("S07 layer handoff", "FAIL",
                f"l1.findings.observed = {observed!r}")
    return ("S07 layer handoff", "PASS", "L0->L1 finding propagated")
