"""S45 — Notification channel kind='script' executes user Python on event.

Tests the script notification transport
(be/internal/notify/transport_script.go): when a workflow run fires
`orchestration.completed`, the notify dispatcher mints a transient
`_notification` agent_session, exports NRFLO_NOTIFY_PAYLOAD_JSON,
NRFLO_SDK_DIR, NRFLO_PROJECT, NRF_SESSION_ID, NRFLO_AGENT_TOKEN, and
NRFLO_SOCKET, and runs the configured `script_code` under the
per-project venv. The script uses the bundled Python SDK to record
that it fired by writing a project_finding.

Expected PASS:
  - workflow run finishes pass
  - within 15 s the project_findings table has `s45_fired` ==
    'orchestration.completed' (proves the script ran and saw the
    payload event_type)
"""

from __future__ import annotations

import time

from lib import db as db_mod
from lib.runtime import (
    Ctx, PASS_STATUSES, Result,
    first_session, make_project, next_id, resolve_model, wait_for_workflow,
)


MODELS_BY_PROVIDER: dict[str, str] = {}

PROMPT = """\
You are an integration-test agent. Run the listed command via the Bash
tool, then stop.

1. Run: `nrflo agent finished`
"""

NOTIFY_SCRIPT = """\
import os, sys
sys.path.insert(0, os.environ["NRFLO_SDK_DIR"])
import nrflo_sdk
c = nrflo_sdk.client()
n = c.notification()
c.project_findings.add("s45_fired", n.event_type)
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
    ctx.client.create_notification_channel(
        pid, wid,
        name=f"s45-{wid}",
        kind="script",
        config={"script_code": NOTIFY_SCRIPT},
        event_types=["orchestration.completed"],
    )

    wfi = ctx.client.run_project_workflow(
        pid, wid, instructions="script notify",
    )["instance_id"]
    wait_for_workflow(ctx, pid, instance_id=wfi)

    sess = first_session(db_mod.agent_sessions_for_instance(ctx.server.home, wfi))
    if sess["status"] not in PASS_STATUSES or sess["result"] != "pass":
        return ("S45 notification script", "FAIL",
                f"status/result = {sess['status']}/{sess['result']}")

    # The notify queue ticks every 5s and the first script run on the
    # project triggers a venv sync, so allow up to 60s.
    deadline = time.monotonic() + 60.0
    last: object = None
    while time.monotonic() < deadline:
        pfs = db_mod.project_findings(ctx.server.home, pid)
        last = pfs.get("s45_fired")
        if last == "orchestration.completed":
            return ("S45 notification script", "PASS",
                    f"project_finding set ({last!r})")
        time.sleep(0.5)
    return ("S45 notification script", "FAIL",
            f"s45_fired = {last!r}, want 'orchestration.completed' "
            "(script never ran or SDK call failed)")
