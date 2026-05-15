"""S13 — Per-project env var propagates to the spawned agent.

Tests:
  - PUT /api/v1/projects/{id}/env-vars/MY_TEST_VAR with value 'red'.
  - The orchestrator loads project env vars at workflow start and
    forwards them to the agent process; bash expansion `$MY_TEST_VAR`
    must yield 'red' inside the spawned shell.

Expected PASS result:
  - agent_sessions.findings.color == 'red'.
"""

from __future__ import annotations

from lib import db as db_mod
from lib.runtime import (
    Ctx, Result, first_session, make_project, next_id, resolve_model, wait_for_workflow,
)


# Per-provider model overrides; empty = use the runner default (e.g. haiku).
MODELS_BY_PROVIDER: dict[str, str] = {}


ENV_NAME = "MY_TEST_VAR"
ENV_VALUE = "red"

PROMPT = """\
You are an integration-test agent. Do EXACTLY what is listed below and
nothing else. Use the Bash tool to run the listed commands in order,
then stop immediately.

1. Run: `nrflo findings add color "$MY_TEST_VAR"`
2. Run: `nrflo agent finished`
"""


def run(ctx: Ctx) -> Result:
    pid, _root = make_project(ctx)
    ctx.client.put_project_env_var(pid, ENV_NAME, ENV_VALUE)

    wid = next_id(ctx, "wf")
    ctx.client.create_workflow(pid, wid, scope_type="project")
    ctx.client.create_agent_def(
        pid, wid, "main",
        model=resolve_model(ctx, MODELS_BY_PROVIDER), layer=0, timeout=5, prompt=PROMPT,
    )
    wfi = ctx.client.run_project_workflow(
        pid, wid, instructions="env var",
    )["instance_id"]
    wait_for_workflow(ctx, pid, instance_id=wfi)

    sess = first_session(
        db_mod.agent_sessions_for_instance(ctx.server.home, wfi))
    color = (sess.get("findings") or {}).get("color")
    if color != ENV_VALUE:
        return ("S13 project env var", "FAIL",
                f"agent saw color={color!r}, want {ENV_VALUE!r}")
    return ("S13 project env var", "PASS",
            f"{ENV_NAME}={ENV_VALUE} reached agent shell")
