"""S31 — take-control on a cli_interactive agent broadcasts viewer-attach.

cli_interactive backends treat take-control as a read-only viewer attach:
the agent keeps running, `agent_sessions.status` stays `running`, and the
spawner broadcasts `agent.viewer_attached` (ws/hub.go:47). The DB kill-and-
flip path is api-mode only and is not covered by this scenario.

Tests:
  - `POST /api/v1/projects/{pid}/workflow/take-control` returns 200 on a
    running cli_interactive session.
  - WS subscriber receives `agent.viewer_attached` for that session.
  - `agent_sessions.status` stays `running` (no flip to user_interactive).
"""

from __future__ import annotations

import importlib.util
import time

from lib import db as db_mod
from lib.runtime import (
    Ctx, Result, make_project, next_id, resolve_model,
)
from lib.ws_client import WSClient


MODELS_BY_PROVIDER: dict[str, str] = {}

POLL_INTERVAL_S = 0.5
WAIT_RUNNING_S = 60.0
WAIT_EVENT_S = 30.0


PROMPT = """\
You are an integration-test agent. Use the Bash tool to run the listed
commands in order, then stop.

1. Run: `sleep 120`
2. Run: `nrflo agent finished`
"""


def _wait_for_running_session(ctx: Ctx, wfi: str) -> dict | None:
    deadline = time.monotonic() + WAIT_RUNNING_S
    while time.monotonic() < deadline:
        sessions = db_mod.agent_sessions_for_instance(ctx.server.home, wfi)
        if sessions and sessions[0].get("status") == "running":
            return sessions[0]
        time.sleep(POLL_INTERVAL_S)
    return None


def run(ctx: Ctx) -> Result:
    if importlib.util.find_spec("websockets") is None:
        return ("S31 take-control viewer-attach", "SKIP",
                "websockets package not installed (pip install websockets)")

    pid, _root = make_project(ctx)
    wid = next_id(ctx, "wf")
    ctx.client.create_workflow(pid, wid, scope_type="project")
    ctx.client.create_agent_def(
        pid, wid, "main",
        model=resolve_model(ctx, MODELS_BY_PROVIDER),
        # Generous per-agent timeout so the agent stays in status=running while
        # take-control is issued; the harness stops the workflow at the end.
        layer=0, timeout=120, prompt=PROMPT,
    )

    with WSClient(ctx.server.base_url, ctx.client._jar) as ws:
        ws.subscribe(pid, since_seq=0)

        wfi = ctx.client.run_project_workflow(
            pid, wid, instructions="take-control viewer attach",
        )["instance_id"]

        sess = _wait_for_running_session(ctx, wfi)
        if not sess:
            try:
                ctx.client.stop_project_workflow(pid, instance_id=wfi)
            except Exception:
                pass
            return ("S31 take-control viewer-attach", "FAIL",
                    "session never reached status=running")

        sid = sess["id"]
        ctx.client.take_control_project(
            pid, workflow=wid, session_id=sid, instance_id=wfi,
        )

        event = ws.wait_for(
            lambda e: (e.get("type") == "agent.viewer_attached"
                       and (e.get("data") or {}).get("session_id") == sid),
            timeout_s=WAIT_EVENT_S,
        )

    if event is None:
        try:
            ctx.client.stop_project_workflow(pid, instance_id=wfi)
        except Exception:
            pass
        return ("S31 take-control viewer-attach", "FAIL",
                f"no agent.viewer_attached event for sid={sid[:8]} "
                f"within {WAIT_EVENT_S}s")

    # Confirm the spawner did NOT flip status — viewer-attach is read-only.
    after = db_mod.agent_sessions_for_instance(ctx.server.home, wfi)
    cur = next((s for s in after if s["id"] == sid), None)
    cur_status = cur.get("status") if cur else None

    try:
        ctx.client.stop_project_workflow(pid, instance_id=wfi)
    except Exception:
        pass

    if cur_status != "running":
        return ("S31 take-control viewer-attach", "FAIL",
                f"status changed during viewer-attach: got {cur_status!r}, "
                "want 'running' (cli_interactive must NOT flip)")

    return ("S31 take-control viewer-attach", "PASS",
            f"viewer attached, status=running (sid={sid[:8]})")
