"""P19 — Python SDK `c.notification()` parses NRFLO_NOTIFY_PAYLOAD_JSON.

`_Notification` (be/internal/sdk/python/nrflo_sdk.py) is only used inside
the notify/transport_script.go dispatch path, where the runtime injects
NRFLO_NOTIFY_PAYLOAD_JSON before invoking the channel's `script_code`.
This scenario exercises the SDK accessor itself from a regular
script-mode agent by setting the env var in-process and reading every
typed property + the raw passthrough.

Expected PASS:
  - agent finishes pass
  - findings record event_type/project_id/workflow/instance_id/reason
    parsed from the payload, and raw_keys equals the JSON key set
"""

from __future__ import annotations

from lib import db as db_mod
from lib.runtime import (
    Ctx, PASS_STATUSES, Result, first_session,
    make_project, next_id, wait_for_workflow,
)
from lib.script_helpers import make_script_agent


CODE = """
import json, os
PAYLOAD = {
    "event_type": "orchestration.completed",
    "project_id": "proj-p19",
    "project_name": "Proj P19",
    "workflow": "wf-p19",
    "instance_id": "wfi-p19",
    "ticket_id": "T-19",
    "ticket_name": "Ticket Nineteen",
    "agent_type": "main",
    "reason": "implicit",
    "workflow_final_result": "ok",
}
os.environ["NRFLO_NOTIFY_PAYLOAD_JSON"] = json.dumps(PAYLOAD)

n = c.notification()
c.findings.add("event_type", n.event_type)
c.findings.add("project_id", n.project_id)
c.findings.add("project_name", n.project_name)
c.findings.add("workflow", n.workflow)
c.findings.add("instance_id", n.instance_id)
c.findings.add("ticket_id", n.ticket_id)
c.findings.add("ticket_name", n.ticket_name)
c.findings.add("agent_type", n.agent_type)
c.findings.add("reason", n.reason)
c.findings.add("summary", n.summary)
c.findings.add("raw_keys", ",".join(sorted(n.raw.keys())))

# Cached: second call must return the same object.
c.findings.add("cached_same", str(c.notification() is n))
c.agent.finished()
"""


def run(ctx: Ctx) -> Result:
    pid, _root = make_project(ctx)
    wid = next_id(ctx, "wf")
    ctx.client.create_workflow(pid, wid, scope_type="project")
    make_script_agent(ctx, pid, wid, "main", code=CODE)
    wfi = ctx.client.run_project_workflow(
        pid, wid, instructions="notification accessor",
    )["instance_id"]
    wait_for_workflow(ctx, pid, instance_id=wfi)

    sess = first_session(db_mod.agent_sessions_for_instance(ctx.server.home, wfi))
    if sess["status"] not in PASS_STATUSES or sess["result"] != "pass":
        return ("P19 notification accessor", "FAIL",
                f"status/result = {sess['status']}/{sess['result']}")
    f = sess.get("findings") or {}
    expected = {
        "event_type": "orchestration.completed",
        "project_id": "proj-p19",
        "project_name": "Proj P19",
        "workflow": "wf-p19",
        "instance_id": "wfi-p19",
        "ticket_id": "T-19",
        "ticket_name": "Ticket Nineteen",
        "agent_type": "main",
        "reason": "implicit",
        # `summary` is the `workflow_final_result` payload key.
        "summary": "ok",
        "cached_same": "True",
    }
    for k, want in expected.items():
        got = f.get(k)
        if got != want:
            return ("P19 notification accessor", "FAIL",
                    f"finding {k!r} = {got!r}, want {want!r}")
    raw_keys = f.get("raw_keys") or ""
    if "event_type" not in raw_keys or "workflow_final_result" not in raw_keys:
        return ("P19 notification accessor", "FAIL",
                f"raw_keys = {raw_keys!r} missing expected payload keys")
    return ("P19 notification accessor", "PASS",
            f"session={sess['id']} parsed {len(expected)} fields")
