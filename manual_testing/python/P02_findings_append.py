"""PS02 — script: findings.add_bulk + findings.append + findings.append_bulk.

Tests SDK methods: `c.findings.add_bulk`, `c.findings.append`,
`c.findings.append_bulk`.

Behaviour reminder:
  - `add` overwrites; `append` concatenates as a list.
  - Values that parse as valid JSON are coerced on storage (e.g. "1"
    becomes int 1). The test uses non-numeric strings so equality
    holds round-trip.

Expected PASS:
  - findings == {"a": "alpha", "b": "beta",
                 "c": ["x", "y"], "d": ["m", "n"]}
"""

from __future__ import annotations

from lib import db as db_mod
from lib.runtime import (
    Ctx, PASS_STATUSES, Result, first_session,
    make_project, next_id, wait_for_workflow,
)
from lib.script_helpers import make_script_agent


CODE = """
c.findings.add_bulk({"a": "alpha", "b": "beta"})
c.findings.append("c", "x")
c.findings.append("c", "y")
c.findings.append_bulk({"d": "m"})
c.findings.append_bulk({"d": "n"})
c.agent.finished()
"""


def run(ctx: Ctx) -> Result:
    pid, _root = make_project(ctx)
    wid = next_id(ctx, "wf")
    ctx.client.create_workflow(pid, wid, scope_type="project")
    make_script_agent(ctx, pid, wid, "main", code=CODE)
    wfi = ctx.client.run_project_workflow(
        pid, wid, instructions="ps02",
    )["instance_id"]
    wait_for_workflow(ctx, pid, instance_id=wfi)

    sess = first_session(db_mod.agent_sessions_for_instance(ctx.server.home, wfi))
    if sess["status"] not in PASS_STATUSES or sess["result"] != "pass":
        return ("PS02 findings.append/bulk", "FAIL",
                f"status/result = {sess['status']}/{sess['result']}")
    f = sess.get("findings") or {}
    if f.get("a") != "alpha" or f.get("b") != "beta":
        return ("PS02 findings.append/bulk", "FAIL", f"add_bulk wrong: {f}")
    # append-result is a list (server stores list-of-values).
    c = f.get("c")
    if not (isinstance(c, list) and "x" in c and "y" in c):
        return ("PS02 findings.append/bulk", "FAIL", f"append c={c!r}")
    d = f.get("d")
    if not (isinstance(d, list) and "m" in d and "n" in d):
        return ("PS02 findings.append/bulk", "FAIL", f"append_bulk d={d!r}")
    return ("PS02 findings.append/bulk", "PASS", f"findings={f}")
