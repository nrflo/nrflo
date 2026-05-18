"""A06 — api-mode findings_add + findings_get builtin tool dispatch.

The model writes one finding via `findings_add`, reads it back via
`findings_get`, then calls `agent_finished`. Verifies the in-process
builtin handlers persist to the unified `findings` table.

Values are non-numeric strings because `normalizeJSONValue`
(be/internal/service/findings.go:69) re-marshals any JSON-parseable
input, so `value="1"` would be stored as the number 1, masking the
string-storage path.

Single-write rather than multi-write because the apirun model often
emits all tool_use blocks in a single assistant turn (parallel tool
use) and the in-process handlers can race on SQLite WAL contention
(visible as 'database is locked' tool_result errors). Single-write keeps
the assertion deterministic; the more elaborate multi-write coverage
is exercised by the Go test suite under controlled scheduling.

Expected PASS:
  - agent_sessions.result == 'pass'
  - findings.alpha == 'red'
"""

from __future__ import annotations

from lib import db as db_mod
from lib.runtime import (
    Ctx, PASS_STATUSES, Result, first_session, make_project, next_id,
    resolve_model, wait_for_workflow,
)


MODELS_BY_PROVIDER: dict[str, str] = {}

PROMPT = """\
You are an integration-test agent running in api-mode. Call these tools
in this exact order, one per turn:
  1. `findings_add` with {"key": "alpha", "value": "red"}
  2. `agent_finished` with {}
Then stop. Do not emit any other text or tool call.
"""


def run(ctx: Ctx) -> Result:
    pid, _root = make_project(ctx)
    wid = next_id(ctx, "wf")
    ctx.client.create_workflow(pid, wid, scope_type="project")
    ctx.client.create_agent_def(
        pid, wid, "main",
        model=resolve_model(ctx, MODELS_BY_PROVIDER),
        layer=0, timeout=90, prompt=PROMPT,
        tools="findings_add,agent_finished",
    )
    wfi = ctx.client.run_project_workflow(
        pid, wid, instructions="api-mode findings",
    )["instance_id"]
    wait_for_workflow(ctx, pid, instance_id=wfi)

    sess = first_session(db_mod.agent_sessions_for_instance(ctx.server.home, wfi))
    if sess["status"] not in PASS_STATUSES or sess["result"] != "pass":
        return ("A06 findings tool", "FAIL",
                f"status/result = {sess['status']}/{sess['result']}")
    findings = sess.get("findings") or {}
    if findings.get("alpha") != "red":
        return ("A06 findings tool", "FAIL",
                f"findings = {findings!r}, want alpha=red")
    return ("A06 findings tool", "PASS",
            f"findings persisted (session={sess['id']})")
