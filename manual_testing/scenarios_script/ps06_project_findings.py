"""PS06 — script: project_findings.{add, add_bulk, append, append_bulk, get,
delete}.

Tests all six methods on `c.project_findings`. Project-scoped findings
persist in the `project_findings` table (separate from session-scoped).
"""

from __future__ import annotations

from lib import db as db_mod
from lib.runtime import (
    Ctx, PASS_STATUSES, Result, first_session,
    make_project, next_id, wait_for_workflow,
)
from lib.script_helpers import make_script_agent


CODE = """
import time

def _retry_busy(fn, *args, **kwargs):
    # Project-wide table sees concurrent writes from other parallel
    # scenarios; SQLITE_BUSY can surface even with busy_timeout. Local
    # retry keeps the test asserting product behaviour, not timing.
    last = None
    for delay in (0.05, 0.1, 0.2, 0.5, 1.0):
        try:
            return fn(*args, **kwargs)
        except nrflo_sdk.NrfloError as e:
            last = e
            if "locked" not in (e.message or "").lower():
                raise
            time.sleep(delay)
    raise last

_retry_busy(c.project_findings.add, "alpha", "one")
_retry_busy(c.project_findings.add_bulk, {"beta": "two", "gamma": "three"})
_retry_busy(c.project_findings.append, "notes", "first")
_retry_busy(c.project_findings.append, "notes", "second")
_retry_busy(c.project_findings.append_bulk, {"tags": "a"})
_retry_busy(c.project_findings.append_bulk, {"tags": "b"})

snap = _retry_busy(c.project_findings.get)
c.findings.add("snap_keys", ",".join(sorted(snap.keys())))

_retry_busy(c.project_findings.delete, "gamma")

after = _retry_busy(c.project_findings.get)
c.findings.add("after_keys", ",".join(sorted(after.keys())))
c.agent.finished()
"""


def run(ctx: Ctx) -> Result:
    pid, _root = make_project(ctx)
    wid = next_id(ctx, "wf")
    ctx.client.create_workflow(pid, wid, scope_type="project")
    make_script_agent(ctx, pid, wid, "main", code=CODE)
    wfi = ctx.client.run_project_workflow(
        pid, wid, instructions="ps06",
    )["instance_id"]
    wait_for_workflow(ctx, pid, instance_id=wfi)

    sess = first_session(db_mod.agent_sessions_for_instance(ctx.server.home, wfi))
    if sess["status"] not in PASS_STATUSES or sess["result"] != "pass":
        return ("PS06 project_findings", "FAIL",
                f"status/result = {sess['status']}/{sess['result']}")
    f = sess.get("findings") or {}
    if "alpha" not in f.get("snap_keys", "") or "gamma" not in f.get("snap_keys", ""):
        return ("PS06 project_findings", "FAIL",
                f"pre-delete snap_keys={f.get('snap_keys')!r}")
    if "gamma" in f.get("after_keys", ""):
        return ("PS06 project_findings", "FAIL",
                f"gamma was not deleted: after_keys={f.get('after_keys')!r}")

    pf = db_mod.project_findings(ctx.server.home, pid)
    if pf.get("alpha") != "one" or pf.get("beta") != "two":
        return ("PS06 project_findings", "FAIL", f"add/add_bulk wrong: {pf}")
    if "gamma" in pf:
        return ("PS06 project_findings", "FAIL", f"delete failed: {pf}")
    notes = pf.get("notes")
    if not (isinstance(notes, list) and "first" in notes and "second" in notes):
        return ("PS06 project_findings", "FAIL", f"append notes={notes!r}")
    tags = pf.get("tags")
    if not (isinstance(tags, list) and "a" in tags and "b" in tags):
        return ("PS06 project_findings", "FAIL", f"append_bulk tags={tags!r}")
    return ("PS06 project_findings", "PASS", f"final pf={pf}")
