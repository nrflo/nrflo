"""A02 — api-mode `agent_fail` terminal signal.

The model calls the `agent_fail` builtin with a reason. The handler
emits `TerminalSignal{Status:"FAIL", Reason: reason}` which the
spawner maps to result=fail / reason=api_error via `mapFinalStatus`
(see be/internal/spawner/backend.go:219 — the agent's reason is
deliberately stomped to 'api_error' for api-mode, unlike cli_interactive
where the socket-side reason persists).

Expected PASS:
  - agent_sessions.result == 'fail'
  - agent_sessions.result_reason == 'api_error'
  - at least one errors row with error_type='agent' for this project
"""

from __future__ import annotations

from lib import db as db_mod
from lib.runtime import (
    Ctx, Result, first_session, make_project, next_id, resolve_model, wait_for_workflow,
)


MODELS_BY_PROVIDER: dict[str, str] = {}

PROMPT = """\
You are an integration-test agent running in api-mode. You have one
available tool: `agent_fail`. Call it once with the JSON input
{"reason": "intentional"}. Do not emit any other text or tool call.
"""


def run(ctx: Ctx) -> Result:
    pid, _root = make_project(ctx)
    wid = next_id(ctx, "wf")
    ctx.client.create_workflow(pid, wid, scope_type="project")
    ctx.client.create_agent_def(
        pid, wid, "main",
        model=resolve_model(ctx, MODELS_BY_PROVIDER),
        layer=0, timeout=60, prompt=PROMPT,
        tools="agent_fail",
    )
    wfi = ctx.client.run_project_workflow(
        pid, wid, instructions="api-mode fail",
    )["instance_id"]
    wait_for_workflow(ctx, pid, instance_id=wfi)

    sess = first_session(db_mod.agent_sessions_for_instance(ctx.server.home, wfi))
    if sess["result"] != "fail":
        return ("A02 agent fail", "FAIL", f"result = {sess['result']!r}")
    if sess.get("result_reason") != "api_error":
        return ("A02 agent fail", "FAIL",
                f"result_reason = {sess.get('result_reason')!r}, "
                "want 'api_error' (mapFinalStatus FAIL -> api_error)")
    # Api-mode agent_fail does NOT record an `agent` error row (unlike
    # the CLI socket path that wraps the fail in errorSvc.RecordError).
    # The orchestrator's layer-policy aggregation records a `workflow`
    # row via markFailed when the FAIL agent breaks the policy.
    errs = db_mod.errors_for_project(ctx.server.home, pid)
    if not any(e["error_type"] == "workflow" for e in errs):
        return ("A02 agent fail", "FAIL", f"no workflow error row (saw {errs})")
    return ("A02 agent fail", "PASS", f"session={sess['id']}")
