"""S26 — Low-context save via resume + carryover on relaunch.

Tests:
  - Set `restart_threshold=100` so the spawner trips the low-context branch
    on the agent's first reported `context_left`. This fires
    `initiateContextSave`, which for claude+codex takes the resume-based
    path (`contextSaveViaResume`): `claude --resume <id>` or
    `codex exec resume <thread_id>`. The resumed turn runs the save prompt,
    writes the `to_resume` finding, calls `nrflo agent continue`, and
    `relaunchForContinuation` spawns a new main-agent session whose
    findings include the carried-over `to_resume`. Opencode/api fall back
    through `shouldUseAgentSave` to the system-agent saver path.

  - With threshold=100 the workflow does not converge: every relaunch
    immediately re-trips. We don't need it to. The test stops the workflow
    as soon as the *main* agent has at least 2 sessions (the second of
    which proves the carryover round-trip worked).

Expected PASS (looking only at main-agent sessions, ignoring the system
`context-saver` rows that may appear when codex's resume falls back):
  - ≥ 2 main agent_sessions rows for the wfi.
  - main[0].result_reason == 'low_context'.
  - main[1].findings.to_resume present and non-empty.
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
DETECT_TIMEOUT_S = 120.0


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


def _main_sessions(ctx: Ctx, wfi: str) -> list[dict]:
    return [
        s for s in db_mod.agent_sessions_for_instance(ctx.server.home, wfi)
        if s.get("agent_type") == MAIN_AGENT
    ]


def run(ctx: Ctx) -> Result:
    pid, _root = make_project(ctx)
    wid = next_id(ctx, "wf")
    ctx.client.create_workflow(pid, wid, scope_type="project")
    ctx.client.create_agent_def(
        pid, wid, MAIN_AGENT,
        model=resolve_model(ctx, MODELS_BY_PROVIDER),
        layer=0, timeout=5, prompt=PROMPT,
        restart_threshold=100,
    )
    wfi = ctx.client.run_project_workflow(
        pid, wid, instructions="context save resume",
    )["instance_id"]

    # Poll until at least 2 main-agent sessions exist (the second of which
    # is the proof that save → resume → relaunch + carryover all worked),
    # OR until the workflow already terminated on its own. Don't rely on
    # wait_for_workflow — with threshold=100 the workflow loops until
    # max_continuations and that's not what's under test.
    deadline = time.monotonic() + DETECT_TIMEOUT_S
    sessions: list[dict] = []
    while time.monotonic() < deadline:
        sessions = _main_sessions(ctx, wfi)
        if len(sessions) >= 2:
            break
        time.sleep(POLL_INTERVAL_S)

    # Stop the workflow regardless of outcome so the harness doesn't sit
    # idle in the relaunch loop.
    try:
        ctx.client.stop_project_workflow(pid, instance_id=wfi)
    except Exception:
        pass

    # Re-read once after stop so a session that was just being written
    # has a chance to settle (findings flush, result_reason update).
    time.sleep(1.0)
    sessions = _main_sessions(ctx, wfi)

    if len(sessions) < 2:
        return ("S26 context save resume", "FAIL",
                f"main agent_sessions count = {len(sessions)}, want >= 2 "
                "(initial + low-context relaunch)")

    first, second = sessions[0], sessions[1]
    if first.get("result_reason") != "low_context":
        return ("S26 context save resume", "FAIL",
                f"main[0] result_reason = {first.get('result_reason')!r}, "
                "want 'low_context'")

    to_resume = (second.get("findings") or {}).get("to_resume")
    if not isinstance(to_resume, str) or to_resume.strip() == "":
        return ("S26 context save resume", "FAIL",
                f"main[1].findings.to_resume = {to_resume!r}, "
                "want non-empty string (carried over by save flow)")

    return ("S26 context save resume", "PASS",
            f"resume+carryover ok (main_sessions={len(sessions)}, "
            f"to_resume_bytes={len(to_resume)})")
