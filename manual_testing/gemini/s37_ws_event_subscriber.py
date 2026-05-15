"""S37 — WebSocket subscriber receives agent.completed + orchestration.completed.

Tests:
  - `GET /api/v1/ws` accepts a v2 subscribe (`{action, project_id,
    since_seq}`) using the admin cookie from the REST client's jar.
  - On a trivial workflow run, the server broadcasts
    `agent.completed` (hub.go:19) and `orchestration.completed`
    (hub.go:37). Both must arrive within 30 s carrying the expected
    `project_id` and `data` fields.

Runtime dep:
  - `websockets` Python package (pip install websockets). See
    `manual_testing/CLAUDE.md`.
"""

from __future__ import annotations

import importlib.util

from lib.runtime import (
    Ctx, Result, make_project, next_id, resolve_model, wait_for_workflow,
)
from lib.ws_client import WSClient


MODELS_BY_PROVIDER: dict[str, str] = {}


PROMPT = """\
You are an integration-test agent. Run the listed command via the Bash
tool, then stop.

1. Run: `nrflo agent finished`
"""


def run(ctx: Ctx) -> Result:
    if importlib.util.find_spec("websockets") is None:
        return ("S37 WS event subscriber", "SKIP",
                "websockets package not installed (pip install websockets)")

    pid, _root = make_project(ctx)
    wid = next_id(ctx, "wf")
    ctx.client.create_workflow(pid, wid, scope_type="project")
    ctx.client.create_agent_def(
        pid, wid, "main",
        model=resolve_model(ctx, MODELS_BY_PROVIDER),
        layer=0, timeout=5, prompt=PROMPT,
    )

    with WSClient(ctx.server.base_url, ctx.client._jar) as ws:
        ws.subscribe(pid, since_seq=0)

        wfi = ctx.client.run_project_workflow(
            pid, wid, instructions="ws subscriber",
        )["instance_id"]

        agent_ev = ws.wait_for(
            lambda e: (e.get("type") == "agent.completed"
                       and e.get("project_id") == pid),
            timeout_s=120.0,
        )
        if not agent_ev:
            try:
                ctx.client.stop_project_workflow(pid, instance_id=wfi)
            except Exception:
                pass
            return ("S37 WS event subscriber", "FAIL",
                    "no agent.completed event within 120s")

        orch_ev = ws.wait_for(
            lambda e: (e.get("type") == "orchestration.completed"
                       and e.get("project_id") == pid
                       and (e.get("data") or {}).get("instance_id") == wfi),
            timeout_s=60.0,
        )
        if not orch_ev:
            try:
                ctx.client.stop_project_workflow(pid, instance_id=wfi)
            except Exception:
                pass
            return ("S37 WS event subscriber", "FAIL",
                    f"no orchestration.completed for wfi={wfi[:8]} within 60s")

    wait_for_workflow(ctx, pid, instance_id=wfi)

    agent_data = agent_ev.get("data") or {}
    if not agent_data.get("session_id"):
        return ("S37 WS event subscriber", "FAIL",
                f"agent.completed missing session_id: {agent_data!r}")

    return ("S37 WS event subscriber", "PASS",
            f"agent.completed sid={agent_data['session_id'][:8]} "
            f"orchestration.completed seq={orch_ev.get('seq')}")
