"""S40 — Agent passes but declarative validation_command fails.

The agent self-reports pass; the first validation command (`false`)
exits non-zero, so the spawner flips the session to fail with
result_reason='validation_failure' and persists a finding whose value
includes the failing command + captured output
(be/internal/spawner/validation.go).

`max_fail_restarts=0` keeps the run from auto-relaunching so we can
assert the failure state cleanly on the original session.

Expected PASS:
  - agent_sessions.result == 'fail'
  - agent_sessions.result_reason == 'validation_failure'
  - findings.validation_failure exists and mentions the failed command
"""

from __future__ import annotations

from lib import db as db_mod
from lib.runtime import (
    Ctx, Result,
    first_session, make_project, next_id, resolve_model, wait_for_workflow,
)


MODELS_BY_PROVIDER: dict[str, str] = {}

PROMPT = """\
You are an integration-test agent. Run the listed command via the Bash
tool, then stop.

1. Run: `nrflo agent finished`
"""


def run(ctx: Ctx) -> Result:
    pid, _root = make_project(ctx)
    wid = next_id(ctx, "wf")
    ctx.client.create_workflow(pid, wid, scope_type="project")
    ctx.client.create_agent_def(
        pid, wid, "main",
        model=resolve_model(ctx, MODELS_BY_PROVIDER),
        layer=0, timeout=5, prompt=PROMPT,
        max_fail_restarts=0,
        validation_commands=["false"],
    )
    wfi = ctx.client.run_project_workflow(
        pid, wid, instructions="validation fail",
    )["instance_id"]
    wait_for_workflow(ctx, pid, instance_id=wfi)

    sess = first_session(db_mod.agent_sessions_for_instance(ctx.server.home, wfi))
    if sess["result"] != "fail":
        return ("S40 validation_commands fail", "FAIL",
                f"result = {sess['result']!r}, want 'fail'")
    if sess.get("result_reason") != "validation_failure":
        return ("S40 validation_commands fail", "FAIL",
                f"result_reason = {sess.get('result_reason')!r}, "
                "want 'validation_failure'")
    findings = sess.get("findings") or {}
    vf = findings.get("validation_failure")
    if vf is None:
        return ("S40 validation_commands fail", "FAIL",
                "missing validation_failure finding "
                f"(keys={sorted(findings)})")
    # Finding stores the failing command + truncated output. Accept either
    # a dict (preferred shape) or a string carrying the command.
    body = vf if isinstance(vf, str) else __import__("json").dumps(vf)
    if "false" not in body:
        return ("S40 validation_commands fail", "FAIL",
                f"validation_failure finding does not mention failing cmd: {vf!r}")
    return ("S40 validation_commands fail", "PASS",
            f"session={sess['id']} reason={sess['result_reason']}")
