"""S29 — Manual restart endpoint links the new session via ancestor_session_id.

Tests (distinct from s18 retry-failed):
  - `POST /api/v1/projects/{pid}/workflow/restart` while an agent is
    running queues a restart signal. The spawner kills the agent and
    spawns a fresh session in the same workflow_instance.
  - The new agent_sessions row carries `ancestor_session_id` pointing
    at the killed session.

Expected PASS result:
  - At least one agent_sessions row exists for the wfi whose
    `ancestor_session_id` equals the first session's id.
"""

from __future__ import annotations

import time

from lib import db as db_mod
from lib.runtime import (
    Ctx, Result, make_project, next_id, resolve_model,
)


MODELS_BY_PROVIDER: dict[str, str] = {}

POLL_INTERVAL_S = 0.5
DETECT_TIMEOUT_S = 120.0


PROMPT = """\
You are an integration-test agent. Use the Bash tool to run the listed
commands in order, then stop.

1. Run: `sleep 60`
2. Run: `nrflo agent finished`
"""


def run(ctx: Ctx) -> Result:
    pid, _root = make_project(ctx)
    wid = next_id(ctx, "wf")
    ctx.client.create_workflow(pid, wid, scope_type="project")
    ctx.client.create_agent_def(
        pid, wid, "main",
        model=resolve_model(ctx, MODELS_BY_PROVIDER),
        layer=0, timeout=5, prompt=PROMPT,
    )
    wfi = ctx.client.run_project_workflow(
        pid, wid, instructions="manual restart",
    )["instance_id"]

    # Wait until the first session exists and is running.
    deadline = time.monotonic() + 60.0
    first_id: str | None = None
    while time.monotonic() < deadline:
        sessions = db_mod.agent_sessions_for_instance(ctx.server.home, wfi)
        if sessions and sessions[0].get("status") == "running":
            first_id = sessions[0]["id"]
            break
        time.sleep(POLL_INTERVAL_S)

    if not first_id:
        try:
            ctx.client.stop_project_workflow(pid, instance_id=wfi)
        except Exception:
            pass
        return ("S29 manual restart", "FAIL",
                "no running session appeared within 60s")

    ctx.client.restart_project_workflow(
        pid, workflow=wid, session_id=first_id, instance_id=wfi,
    )

    # Poll until a session with ancestor_session_id == first_id appears.
    deadline = time.monotonic() + DETECT_TIMEOUT_S
    matched: dict | None = None
    while time.monotonic() < deadline:
        sessions = db_mod.agent_sessions_for_instance(ctx.server.home, wfi)
        matched = next(
            (s for s in sessions if s.get("ancestor_session_id") == first_id),
            None,
        )
        if matched:
            break
        time.sleep(POLL_INTERVAL_S)

    try:
        ctx.client.stop_project_workflow(pid, instance_id=wfi)
    except Exception:
        pass

    if not matched:
        return ("S29 manual restart", "FAIL",
                f"no session linked back to {first_id[:8]} within "
                f"{DETECT_TIMEOUT_S}s")

    return ("S29 manual restart", "PASS",
            f"new session={matched['id'][:8]} ancestor={first_id[:8]}")
