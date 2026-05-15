"""PS05 — script: findings.get(agent_type=..., layer=...) cross-agent reads.

Tests SDK method: `c.findings.get(agent_type=...)`, `c.findings.get(layer=N)`.

L0 ("producer") writes `topic=apples`. L1 ("reader") reads via both
the agent_type filter and the layer-keyed map, then records what it
saw into its own findings so we can assert.
"""

from __future__ import annotations

from lib import db as db_mod
from lib.runtime import (
    Ctx, PASS_STATUSES, Result, make_project, next_id, wait_for_workflow,
)
from lib.script_helpers import make_script_agent


L0_CODE = """
c.findings.add("topic", "apples")
c.agent.finished()
"""

L1_CODE = """
by_agent = c.findings.get(agent_type="producer")
by_layer = c.findings.get(layer=0)

c.findings.add("read_by_agent", str(by_agent.get("topic")))
c.findings.add("read_by_layer", str(by_layer))
c.agent.finished()
"""


def run(ctx: Ctx) -> Result:
    pid, _root = make_project(ctx)
    wid = next_id(ctx, "wf")
    ctx.client.create_workflow(pid, wid, scope_type="project")
    make_script_agent(ctx, pid, wid, "producer", code=L0_CODE, layer=0)
    make_script_agent(ctx, pid, wid, "reader",   code=L1_CODE, layer=1)
    wfi = ctx.client.run_project_workflow(
        pid, wid, instructions="ps05",
    )["instance_id"]
    wait_for_workflow(ctx, pid, instance_id=wfi)

    sessions = db_mod.agent_sessions_for_instance(ctx.server.home, wfi)
    by_type = {s["agent_type"]: s for s in sessions}
    if "producer" not in by_type or "reader" not in by_type:
        return ("PS05 layer get", "FAIL",
                f"missing sessions: {list(by_type)}")
    if by_type["reader"].get("result") != "pass":
        return ("PS05 layer get", "FAIL",
                f"reader result = {by_type['reader'].get('result')!r}")
    rf = by_type["reader"].get("findings") or {}
    if "apples" not in rf.get("read_by_agent", ""):
        return ("PS05 layer get", "FAIL",
                f"read_by_agent={rf.get('read_by_agent')!r}")
    # layer=0 returns {agent_type: findings_dict|null}
    if "producer" not in rf.get("read_by_layer", "") or "apples" not in rf.get("read_by_layer", ""):
        return ("PS05 layer get", "FAIL",
                f"read_by_layer={rf.get('read_by_layer')!r}")
    return ("PS05 layer get", "PASS", "agent_type+layer reads verified")
