"""S07 — Prompt template expansion (L0 → L1).

One 2-layer project-scope workflow that exercises both prompt-template
placeholders the spawner expands at L1 render time. Folds the former
s07 (#{FINDINGS:agent:key}) and s15 (#{PRIOR_LAYER_FINDINGS}) into a
single workflow run.

Sub-assertions:
  - [findings:l0:k] L1 prompt contains '42' (specific-finding template
    expanded) and L1.findings.observed == '42' (proves the resolved
    value is also passed through Bash exec, not just the prompt string)
  - [prior_layer]   L1 prompt contains 'prior_value_99'
                    (#{PRIOR_LAYER_FINDINGS} expanded)
  - [sessions]      L0 and L1 sessions both present, L1.result == 'pass'
"""

from __future__ import annotations

from lib import db as db_mod
from lib.runtime import (
    Ctx, Result, make_project, next_id, resolve_model, wait_for_workflow,
)


MODELS_BY_PROVIDER: dict[str, str] = {}

L0_PROMPT = """\
You are an integration-test agent. Do EXACTLY what is listed below and
nothing else. Use the Bash tool to run the listed commands in order,
then stop immediately.

1. Run: `nrflo findings add handoff_value 42`
2. Run: `nrflo findings add prior_key prior_value_99`
3. Run: `nrflo agent finished`
"""

L1_PROMPT = """\
You are an integration-test agent. Do EXACTLY what is listed below and
nothing else. Use the Bash tool to run the listed commands in order,
then stop immediately.

The previous layer wrote: #{FINDINGS:l0:handoff_value}

Prior layer findings follow:
#{PRIOR_LAYER_FINDINGS}

1. Run: `nrflo findings add observed #{FINDINGS:l0:handoff_value}`
2. Run: `nrflo agent finished`
"""

NAME = "S07 template expansion"


def run(ctx: Ctx) -> Result:
    pid, _root = make_project(ctx)
    wid = next_id(ctx, "wf")
    ctx.client.create_workflow(pid, wid, scope_type="project")
    ctx.client.create_agent_def(
        pid, wid, "l0",
        model=resolve_model(ctx, MODELS_BY_PROVIDER),
        layer=0, timeout=5, prompt=L0_PROMPT)
    ctx.client.create_agent_def(
        pid, wid, "l1",
        model=resolve_model(ctx, MODELS_BY_PROVIDER),
        layer=1, timeout=5, prompt=L1_PROMPT)
    wfi = ctx.client.run_project_workflow(
        pid, wid, instructions="template expansion",
    )["instance_id"]
    wait_for_workflow(ctx, pid, instance_id=wfi)

    sessions = db_mod.agent_sessions_for_instance(ctx.server.home, wfi)
    by_type = {s["agent_type"]: s for s in sessions}

    fails: list[str] = []

    if "l0" not in by_type or "l1" not in by_type:
        return (NAME, "FAIL", f"[sessions] missing, got {list(by_type)}")

    l1 = by_type["l1"]
    if l1.get("result") != "pass":
        fails.append(f"[sessions] l1.result = {l1.get('result')!r}")

    l1_prompt = l1.get("prompt") or ""
    if "42" not in l1_prompt:
        fails.append("[findings:l0:k] '42' missing from L1 prompt")
    observed = (l1.get("findings") or {}).get("observed")
    if str(observed) != "42":
        fails.append(f"[findings:l0:k] l1.findings.observed = {observed!r}")

    if "prior_value_99" not in l1_prompt:
        fails.append(
            "[prior_layer] 'prior_value_99' missing from L1 prompt")

    if fails:
        return (NAME, "FAIL", "; ".join(fails))
    return (NAME, "PASS",
            "#{FINDINGS:l0:handoff_value} and #{PRIOR_LAYER_FINDINGS} both expanded")
