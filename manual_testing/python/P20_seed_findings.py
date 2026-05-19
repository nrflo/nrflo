"""P20 — script: c.seed_findings() returns RunRequest.SeedFindings filtered.

Tests SDK method: `c.seed_findings()` (be/internal/sdk/python/nrflo_sdk.py:349)
and the `script.context` socket handler's seed_findings filtering
(be/internal/socket/handler_script_context.go:82) which strips
`user_instructions` (already surfaced separately) and underscore-prefixed
orchestrator-internal keys.

Pre-seeds 3 keys via `POST /api/v1/projects/{id}/workflow/run` body
`seed_findings`:
  - `ext_id` → user key, must round-trip to the SDK
  - `user_instructions` → filtered (surfaced via c.user_instructions())
  - `_callback` → filtered (orchestrator-internal prefix)

Expected PASS:
  - agent_sessions.result == 'pass'
  - sess.findings.seed_keys == "ext_id" (sorted, comma-joined)
  - sess.findings.ext_value == "spec-42"
"""

from __future__ import annotations

from lib import db as db_mod
from lib.runtime import (
    Ctx, PASS_STATUSES, Result, first_session,
    make_project, next_id, wait_for_workflow,
)
from lib.script_helpers import make_script_agent


CODE = """
seeds = c.seed_findings()
c.findings.add("seed_keys", ",".join(sorted(seeds.keys())))
c.findings.add("ext_value", str(seeds.get("ext_id")))
c.findings.add("user_instr", c.user_instructions())
c.agent.finished()
"""


def run(ctx: Ctx) -> Result:
    pid, _root = make_project(ctx)
    wid = next_id(ctx, "wf")
    ctx.client.create_workflow(pid, wid, scope_type="project")
    make_script_agent(ctx, pid, wid, "main", code=CODE)
    wfi = ctx.client.run_project_workflow(
        pid, wid, instructions="p20-user-instructions",
        seed_findings={
            "ext_id": "spec-42",
            "user_instructions": "should-be-filtered",
            "_callback": "internal",
        },
    )["instance_id"]
    wait_for_workflow(ctx, pid, instance_id=wfi)

    sess = first_session(db_mod.agent_sessions_for_instance(ctx.server.home, wfi))
    if sess["status"] not in PASS_STATUSES or sess["result"] != "pass":
        return ("P20 seed_findings", "FAIL",
                f"status/result = {sess['status']}/{sess['result']}")
    f = sess.get("findings") or {}
    if f.get("seed_keys") != "ext_id":
        return ("P20 seed_findings", "FAIL",
                f"seed_keys = {f.get('seed_keys')!r}, want 'ext_id' "
                f"(user_instructions + _callback must be filtered)")
    if f.get("ext_value") != "spec-42":
        return ("P20 seed_findings", "FAIL",
                f"ext_value = {f.get('ext_value')!r}, want 'spec-42'")
    if f.get("user_instr") != "p20-user-instructions":
        return ("P20 seed_findings", "FAIL",
                f"user_instr = {f.get('user_instr')!r} "
                "(c.user_instructions() should still work)")
    return ("P20 seed_findings", "PASS",
            f"seed_findings={{ext_id: spec-42}} filtered correctly")
