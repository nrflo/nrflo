"""PS03 — script: findings.get(key/keys) — own-session reads.

Tests SDK method: `c.findings.get(key=...)`, `c.findings.get(keys=[...])`.

Server behaviour reminders (see `extractKeys` in service/findings.go):
  - `key="x"` (single key) returns the bare value, NOT a wrapping dict.
  - `keys=[...]` with multiple keys returns `{k: v}`.
  - No filter returns the full session findings dict.
"""

from __future__ import annotations

from lib import db as db_mod
from lib.runtime import (
    Ctx, PASS_STATUSES, Result, first_session,
    make_project, next_id, wait_for_workflow,
)
from lib.script_helpers import make_script_agent


CODE = """
c.findings.add_bulk({"a": "alpha", "b": "beta", "c": "gamma"})

# Single-key read — returns the bare value
one = c.findings.get(key="a")
# Multi-key read — returns a {k: v} dict
many = c.findings.get(keys=["a", "c"])
# No-arg read — returns the full dict
all_own = c.findings.get()

c.findings.add("observed_one", str(one))
c.findings.add("observed_many", str(sorted(many.items())))
c.findings.add("has_b_in_all", str("b" in all_own))
c.agent.finished()
"""


def run(ctx: Ctx) -> Result:
    pid, _root = make_project(ctx)
    wid = next_id(ctx, "wf")
    ctx.client.create_workflow(pid, wid, scope_type="project")
    make_script_agent(ctx, pid, wid, "main", code=CODE)
    wfi = ctx.client.run_project_workflow(
        pid, wid, instructions="ps03",
    )["instance_id"]
    wait_for_workflow(ctx, pid, instance_id=wfi)

    sess = first_session(db_mod.agent_sessions_for_instance(ctx.server.home, wfi))
    if sess["status"] not in PASS_STATUSES or sess["result"] != "pass":
        return ("PS03 findings.get filters", "FAIL",
                f"status/result = {sess['status']}/{sess['result']}")
    f = sess.get("findings") or {}
    # Single-key returns the bare value 'alpha'.
    if f.get("observed_one") != "alpha":
        return ("PS03 findings.get filters", "FAIL",
                f"observed_one={f.get('observed_one')!r}")
    # Multi-key returns {"a":"alpha","c":"gamma"} — both keys present.
    obs_many = f.get("observed_many", "")
    if "'a', 'alpha'" not in obs_many or "'c', 'gamma'" not in obs_many:
        return ("PS03 findings.get filters", "FAIL",
                f"observed_many={obs_many!r}")
    if f.get("has_b_in_all") != "True":
        return ("PS03 findings.get filters", "FAIL",
                f"has_b_in_all={f.get('has_b_in_all')!r}")
    return ("PS03 findings.get filters", "PASS",
            f"single/multi/all readback verified")
