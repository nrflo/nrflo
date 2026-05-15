"""S24 — Finished sessions surface in /agent-session-logs REST endpoint.

Tests:
  - After a project-scope workflow completes, the GET /api/v1/agent-session-logs
    endpoint returns the finished session with the joined fields the UI
    expects (workflow_id, scope_type, execution_mode, duration_sec).

Expected PASS result:
  - The response includes a row whose session_id matches the finished
    agent's id, and that row carries non-empty workflow_id and a numeric
    duration_sec ≥ 0.
"""

from __future__ import annotations

from lib import db as db_mod
from lib.runtime import (
    Ctx, Result, first_session, make_project, next_id, resolve_model, wait_for_workflow,
)


# Per-provider model overrides; empty = use the runner default (e.g. haiku).
MODELS_BY_PROVIDER: dict[str, str] = {}


PROMPT = """\
You are an integration-test agent. Do EXACTLY what is listed below and
nothing else. Use the Bash tool to run the listed command, then stop.

1. Run: `nrflo agent finished`
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
        pid, wid, instructions="logs api",
    )["instance_id"]
    wait_for_workflow(ctx, pid, instance_id=wfi)

    sess = first_session(db_mod.agent_sessions_for_instance(ctx.server.home, wfi))
    resp = ctx.client._request(
        "GET", "/api/v1/agent-session-logs?per_page=100", project=pid,
    )
    rows = resp.get("rows") or resp.get("sessions") or resp.get("items") or []
    if not isinstance(rows, list):
        # Some servers return the array under a different key — also try top-level.
        if isinstance(resp, list):
            rows = resp
        else:
            return ("S24 agent-session-logs", "FAIL",
                    f"unexpected response shape: keys={list(resp)[:6]}")
    match = next((r for r in rows if r.get("session_id") == sess["id"]
                  or r.get("id") == sess["id"]), None)
    if not match:
        return ("S24 agent-session-logs", "FAIL",
                f"session {sess['id']} not in logs response "
                f"(saw {len(rows)} rows)")
    if not match.get("workflow_id"):
        return ("S24 agent-session-logs", "FAIL",
                f"workflow_id missing/empty on row: {match}")
    dur = match.get("duration_sec")
    if dur is None or dur < 0:
        return ("S24 agent-session-logs", "FAIL",
                f"duration_sec={dur!r}")
    return ("S24 agent-session-logs", "PASS",
            f"row found, workflow_id={match.get('workflow_id')!r}, "
            f"duration_sec={dur}")
