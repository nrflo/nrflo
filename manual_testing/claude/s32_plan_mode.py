"""S32 — plan_mode pre-step creates a user_interactive L0 session.

Tests:
  - `POST /api/v1/projects/{pid}/workflow/run` with `plan_mode=true`
    triggers the orchestrator's plan-mode pre-step
    (`orchestrator_interactive.go:setupInteractivePreStep`), which
    creates an L0 agent session in `user_interactive` status before
    any normal layer execution starts.
  - REST/DB only — the PTY drive-through (plan-file write, exit) is
    not exercised; we only assert that the pre-step session arrives
    and is in the expected state, then stop the workflow.
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
You are an integration-test agent. Run `nrflo agent finished` and stop.
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
    run = ctx.client.run_project_workflow(
        pid, wid, instructions="plan", plan_mode=True,
    )
    wfi = run["instance_id"]

    deadline = time.monotonic() + DETECT_TIMEOUT_S
    sess: dict | None = None
    while time.monotonic() < deadline:
        sessions = db_mod.agent_sessions_for_instance(ctx.server.home, wfi)
        if sessions and sessions[0].get("status") == "user_interactive":
            sess = sessions[0]
            break
        time.sleep(POLL_INTERVAL_S)

    try:
        ctx.client.stop_project_workflow(pid, instance_id=wfi)
    except Exception:
        pass

    if not sess:
        cur = db_mod.agent_sessions_for_instance(ctx.server.home, wfi)
        return ("S32 plan_mode", "FAIL",
                f"no user_interactive pre-step session within "
                f"{DETECT_TIMEOUT_S}s; "
                f"got={[s.get('status') for s in cur]}")

    return ("S32 plan_mode", "PASS",
            f"pre-step session={sess['id'][:8]} status=user_interactive")
