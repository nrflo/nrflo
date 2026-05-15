"""S27 — Low-context save via the system-agent saver path.

Tests:
  - The opencode adapter returns `SupportsResume() == false`, so
    `shouldUseAgentSave` (`be/internal/spawner/context_save.go:74`)
    routes the low-context save through `contextSaveViaAgent`: a fresh
    haiku `context-saver` system agent reads message history and writes
    the `to_resume` finding. The relaunched main agent inherits that
    finding via the `low-context` injectable.
  - Mirrors s26 (resume path) but for the agent-saver branch. opencode
    is the only stock provider that takes this branch — claude/codex
    both support resume.

Expected PASS (looking only at main-agent sessions):
  - ≥ 2 main agent_sessions rows for the wfi.
  - main[0].result_reason == 'low_context'.
  - main[1].findings.to_resume is non-empty.
  - At least one `context-saver` agent_sessions row exists for the wfi
    (proof the agent-saver branch fired, not the resume branch).
"""

from __future__ import annotations

import time

from lib import db as db_mod
from lib.runtime import (
    Ctx, Result, make_project, next_id, resolve_model,
)


MODELS_BY_PROVIDER: dict[str, str] = {}

MAIN_AGENT = "main"
POLL_INTERVAL_S = 0.5
# Mirrors s26's headroom: the context-saver runs a real haiku turn (with
# a possible stall-restart) before relaunch. 300s keeps slow machines green.
DETECT_TIMEOUT_S = 300.0


PROMPT = """\
You are an integration-test agent. Behavior depends on whether the
finding `to_resume` is already set on YOUR session.

First, run: `nrflo findings get to_resume`

- If the output shows a non-empty value, this is the RELAUNCHED attempt
  (after a low-context save). Run, in order:
    1. `nrflo agent finished`
  Then stop.

- If the output is empty or shows "no finding", this is the FIRST
  attempt. Run these commands in order, each as its own turn — do NOT
  combine them — then stop:
    1. `nrflo findings add step1 done`
    2. `nrflo findings add step2 done`
    3. `nrflo findings add step3 done`
    4. `nrflo findings add step4 done`
    5. `nrflo agent finished`
"""


def _sessions_by_type(ctx: Ctx, wfi: str, agent_type: str) -> list[dict]:
    return [
        s for s in db_mod.agent_sessions_for_instance(ctx.server.home, wfi)
        if s.get("agent_type") == agent_type
    ]


def run(ctx: Ctx) -> Result:
    pid, _root = make_project(ctx)
    wid = next_id(ctx, "wf")
    ctx.client.create_workflow(pid, wid, scope_type="project")
    ctx.client.create_agent_def(
        pid, wid, MAIN_AGENT,
        model=resolve_model(ctx, MODELS_BY_PROVIDER),
        # Generous timeout — the agent must reach its first context_left
        # report (which trips restart_threshold=100) before the per-agent
        # timer would otherwise kill it.
        layer=0, timeout=120, prompt=PROMPT,
        restart_threshold=100,
    )
    wfi = ctx.client.run_project_workflow(
        pid, wid, instructions="context save agent saver",
    )["instance_id"]

    deadline = time.monotonic() + DETECT_TIMEOUT_S
    mains: list[dict] = []
    while time.monotonic() < deadline:
        mains = _sessions_by_type(ctx, wfi, MAIN_AGENT)
        if len(mains) >= 2:
            break
        time.sleep(POLL_INTERVAL_S)

    try:
        ctx.client.stop_project_workflow(pid, instance_id=wfi)
    except Exception:
        pass

    time.sleep(1.0)
    mains = _sessions_by_type(ctx, wfi, MAIN_AGENT)
    savers = _sessions_by_type(ctx, wfi, "context-saver")

    if len(mains) < 2:
        return ("S27 context save (agent saver)", "FAIL",
                f"main agent_sessions count = {len(mains)}, want >= 2")
    first, second = mains[0], mains[1]
    if first.get("result_reason") != "low_context":
        return ("S27 context save (agent saver)", "FAIL",
                f"main[0] result_reason = {first.get('result_reason')!r}, "
                "want 'low_context'")
    if not savers:
        return ("S27 context save (agent saver)", "FAIL",
                "no context-saver agent_sessions row — agent-saver "
                "branch did not fire (resume path taken instead?)")
    to_resume = (second.get("findings") or {}).get("to_resume")
    if not isinstance(to_resume, str) or to_resume.strip() == "":
        return ("S27 context save (agent saver)", "FAIL",
                f"main[1].findings.to_resume = {to_resume!r}, "
                "want non-empty string")

    return ("S27 context save (agent saver)", "PASS",
            f"main_sessions={len(mains)} savers={len(savers)} "
            f"to_resume_bytes={len(to_resume)}")
