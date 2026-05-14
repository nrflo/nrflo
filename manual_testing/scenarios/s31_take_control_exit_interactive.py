"""S31 — take-control + exit-interactive session-status transitions.

Tests:
  - `POST /api/v1/projects/{pid}/workflow/take-control` on a running
    batch session kills the agent and flips
    `agent_sessions.status` to `user_interactive`.
  - `POST .../workflow/exit-interactive` flips status to
    `interactive_completed`.
  - REST/DB only — no PTY drive-through (the harness does not attach
    a terminal). The PTY-bound behavior (resize, input relay) is not
    covered here.

Skips:
  - `cli-interactive` mode — the session is already PTY-bound, so
    take-control is not the transition under test.
"""

from __future__ import annotations

import time

from lib import db as db_mod
from lib.runtime import (
    Ctx, Result, make_project, next_id, resolve_model,
)


MODELS_BY_PROVIDER: dict[str, str] = {}

POLL_INTERVAL_S = 0.5
DETECT_TIMEOUT_S = 30.0


PROMPT = """\
You are an integration-test agent. Use the Bash tool to run the listed
commands in order, then stop.

1. Run: `sleep 60`
2. Run: `nrflo agent finished`
"""


def _wait_for_status(ctx: Ctx, wfi: str, want: str,
                     timeout_s: float = DETECT_TIMEOUT_S) -> dict | None:
    deadline = time.monotonic() + timeout_s
    while time.monotonic() < deadline:
        sessions = db_mod.agent_sessions_for_instance(ctx.server.home, wfi)
        if sessions and sessions[0].get("status") == want:
            return sessions[0]
        time.sleep(POLL_INTERVAL_S)
    return None


def run(ctx: Ctx) -> Result:
    if ctx.mode == "cli-interactive":
        return ("S31 take-control exit-interactive", "SKIP",
                "cli-interactive starts in PTY mode; take-control transition n/a")
    pid, _root = make_project(ctx)
    wid = next_id(ctx, "wf")
    ctx.client.create_workflow(pid, wid, scope_type="project")
    ctx.client.create_agent_def(
        pid, wid, "main",
        model=resolve_model(ctx, MODELS_BY_PROVIDER),
        layer=0, timeout=5, prompt=PROMPT,
    )
    wfi = ctx.client.run_project_workflow(
        pid, wid, instructions="take-control",
    )["instance_id"]

    sess = _wait_for_status(ctx, wfi, "running", 60.0)
    if not sess:
        try:
            ctx.client.stop_project_workflow(pid, instance_id=wfi)
        except Exception:
            pass
        return ("S31 take-control exit-interactive", "FAIL",
                "session never reached status=running")

    sid = sess["id"]
    ctx.client.take_control_project(
        pid, workflow=wid, session_id=sid, instance_id=wfi,
    )

    flipped = _wait_for_status(ctx, wfi, "user_interactive")
    if not flipped or flipped["id"] != sid:
        try:
            ctx.client.stop_project_workflow(pid, instance_id=wfi)
        except Exception:
            pass
        cur = db_mod.agent_sessions_for_instance(ctx.server.home, wfi)
        return ("S31 take-control exit-interactive", "FAIL",
                f"status did not flip to user_interactive within "
                f"{DETECT_TIMEOUT_S}s; got={cur[0].get('status') if cur else None}")

    ctx.client.exit_interactive_project(
        pid, workflow=wid, session_id=sid,
    )

    done = _wait_for_status(ctx, wfi, "interactive_completed")
    try:
        ctx.client.stop_project_workflow(pid, instance_id=wfi)
    except Exception:
        pass
    if not done or done["id"] != sid:
        cur = db_mod.agent_sessions_for_instance(ctx.server.home, wfi)
        return ("S31 take-control exit-interactive", "FAIL",
                f"status did not flip to interactive_completed within "
                f"{DETECT_TIMEOUT_S}s; got={cur[0].get('status') if cur else None}")

    return ("S31 take-control exit-interactive", "PASS",
            f"running → user_interactive → interactive_completed (sid={sid[:8]})")
