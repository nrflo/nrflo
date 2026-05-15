"""S15 — #{PRIOR_LAYER_FINDINGS} template expansion.

Tests:
  - L0 writes a finding; L1's prompt uses `#{PRIOR_LAYER_FINDINGS}`.
  - The spawner expands the placeholder with the previous layer's
    findings before sending the prompt to the CLI.

Expected PASS result:
  - L1 agent_sessions.prompt column contains the L0 finding value
    ('prior_value_99'), proving the template variable expanded.
"""

from __future__ import annotations

from lib import db as db_mod
from lib.runtime import Ctx, Result, make_project, next_id, resolve_model, wait_for_workflow


# Per-provider model overrides; empty = use the runner default (e.g. haiku).
MODELS_BY_PROVIDER: dict[str, str] = {}


L0_PROMPT = """\
You are an integration-test agent. Do EXACTLY what is listed below and
nothing else. Use the Bash tool to run the listed commands in order,
then stop immediately.

1. Run: `nrflo findings add prior_key prior_value_99`
2. Run: `nrflo agent finished`
"""

L1_PROMPT = """\
You are an integration-test agent. Do EXACTLY what is listed below and
nothing else. Use the Bash tool to run the listed command, then stop.

Prior layer findings follow:
#{PRIOR_LAYER_FINDINGS}

1. Run: `nrflo agent finished`
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
        pid, wid, instructions="prior_layer_findings",
    )["instance_id"]
    wait_for_workflow(ctx, pid, instance_id=wfi)

    sessions = db_mod.agent_sessions_for_instance(ctx.server.home, wfi)
    l1 = next((s for s in sessions if s["agent_type"] == "l1"), None)
    if not l1:
        return ("S15 prior_layer_findings", "FAIL", "L1 session missing")
    if "prior_value_99" not in (l1.get("prompt") or ""):
        return ("S15 prior_layer_findings", "FAIL",
                "L0 value not inlined into L1 prompt")
    return ("S15 prior_layer_findings", "PASS",
            "#{PRIOR_LAYER_FINDINGS} expanded")
