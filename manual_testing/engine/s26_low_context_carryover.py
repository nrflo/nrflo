"""S26 — Low-context relaunch with context carryover.

Tests the present-state save flow (`initiateContextSave`,
be/internal/spawner/context_save.go): on a low-context trip the spawner
kills the agent, forces one autonomous refinery fold, and then EITHER a
fresh slot digest at (workflow_instance_id, node_id) covers the session —
the context-saver spawn is skipped and the relaunch prompt reads the
digest — OR it falls back to the `context-saver` system agent, which
writes the `to_resume` finding. There is no provider-native resume path
(`claude --resume` / `codex exec resume` for saves was removed in favor
of the refinery).

Setup: `restart_threshold=100` so the spawner trips the low-context
branch on the agent's first reported `context_left`. With that threshold
the workflow never converges (every relaunch immediately re-trips) — the
test stops it as soon as the *main* agent has at least 2 sessions.

Expected PASS (looking only at main-agent sessions, ignoring system
`context-saver`/`_refinery` rows):
  - >= 2 main agent_sessions rows for the wfi.
  - main[0].result_reason == 'low_context'.
  - Carryover evidence, either branch: a non-empty
    refinery_autonomous_digests row for the wfi, OR a non-empty
    `to_resume` finding on a main session.
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
# The agent-saver fallback runs a real model turn, with one stall-restart
# possible. 300s headroom keeps slow CI/dev machines green.
DETECT_TIMEOUT_S = 300.0


PROMPT = """\
You are an integration-test agent. Run these commands in order, each as
its own turn — do NOT combine them — then stop:
  1. the `findings_add` tool (key=step1, value=done)
  2. the `findings_add` tool (key=step2, value=done)
  3. the `findings_add` tool (key=step3, value=done)
  4. the `findings_add` tool (key=step4, value=done)
  5. the `agent_finished` tool
"""


def _main_sessions(ctx: Ctx, wfi: str) -> list[dict]:
    return [
        s for s in db_mod.agent_sessions_for_instance(ctx.server.home, wfi)
        if s.get("agent_type") == MAIN_AGENT
    ]


def _slot_digest(ctx: Ctx, wfi: str) -> str:
    rows = db_mod.refinery_slot_digests(ctx.server.home, wfi)
    return "".join(r.get("content") or "" for r in rows)


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
        pid, wid, instructions="low context carryover",
    )["instance_id"]

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
        return ("S26 low-context carryover", "FAIL",
                f"main agent_sessions count = {len(sessions)}, want >= 2 "
                "(initial + low-context relaunch)")

    first = sessions[0]
    if first.get("result_reason") != "low_context":
        return ("S26 low-context carryover", "FAIL",
                f"main[0] result_reason = {first.get('result_reason')!r}, "
                "want 'low_context'")

    digest = _slot_digest(ctx, wfi)
    to_resume = ""
    for s in sessions:
        val = (s.get("findings") or {}).get("to_resume")
        if isinstance(val, str) and val.strip():
            to_resume = val
            break

    if not digest.strip() and not to_resume:
        return ("S26 low-context carryover", "FAIL",
                "no carryover evidence: refinery_autonomous_digests empty "
                "for the wfi AND no non-empty to_resume finding on any "
                "main session")

    branch = "digest" if digest.strip() else "to_resume"
    return ("S26 low-context carryover", "PASS",
            f"carryover ok via {branch} (main_sessions={len(sessions)}, "
            f"digest_bytes={len(digest)}, to_resume_bytes={len(to_resume)})")
