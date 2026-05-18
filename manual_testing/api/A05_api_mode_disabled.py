"""A05 — api-mode disabled global gate.

Flips `api_mode_enabled` back to `false`, attempts to spawn an api-mode
agent. `prepareSpawn` (be/internal/spawner/spawner.go:971) returns
`api_mode_disabled` before any agent_session row is created, so the
orchestrator's layer aggregator counts the agent as failed, the
default `any` pass_policy fails for `0/1 passed, 1 required`, and
`markFailed` marks the instance failed + records an errors row with
`error_type='workflow'`. Restores the flag at end so subsequent
scenarios still work.

Expected PASS:
  - workflow_instance.status == 'failed'
  - zero agent_sessions rows for the wfi (prepareSpawn rejected pre-row)
  - at least one errors row with error_type='workflow' for the project
"""

from __future__ import annotations

from lib import db as db_mod
from lib.runtime import (
    Ctx, Result, make_project, next_id, resolve_model, wait_for_workflow,
)


MODELS_BY_PROVIDER: dict[str, str] = {}

PROMPT = """\
You are an integration-test agent. Call `agent_finished` with {} and stop.
"""


def run(ctx: Ctx) -> Result:
    ctx.client.set_global_setting("api_mode_enabled", False)
    try:
        pid, _root = make_project(ctx)
        wid = next_id(ctx, "wf")
        ctx.client.create_workflow(pid, wid, scope_type="project")
        ctx.client.create_agent_def(
            pid, wid, "main",
            model=resolve_model(ctx, MODELS_BY_PROVIDER),
            layer=0, timeout=30, prompt=PROMPT,
            tools="agent_finished",
        )
        wfi = ctx.client.run_project_workflow(
            pid, wid, instructions="api-mode disabled",
        )["instance_id"]
        try:
            wait_for_workflow(ctx, pid, instance_id=wfi)
        except TimeoutError:
            return ("A05 api_mode_disabled", "FAIL",
                    "workflow did not terminate within RUN_TIMEOUT_S")

        inst = db_mod.workflow_instance(ctx.server.home, wfi)
        if not inst or inst.get("status") != "failed":
            return ("A05 api_mode_disabled", "FAIL",
                    f"workflow_instance.status = "
                    f"{(inst or {}).get('status')!r}, want 'failed'")
        sessions = db_mod.agent_sessions_for_instance(ctx.server.home, wfi)
        if sessions:
            return ("A05 api_mode_disabled", "FAIL",
                    f"agent_sessions count = {len(sessions)}, "
                    "want 0 (prepareSpawn must reject pre-row)")
        errs = db_mod.errors_for_project(ctx.server.home, pid)
        wf_errs = [e for e in errs if e.get("error_type") == "workflow"]
        if not wf_errs:
            return ("A05 api_mode_disabled", "FAIL",
                    f"no workflow-type errors row "
                    f"(saw {[e.get('error_type') for e in errs]})")
        return ("A05 api_mode_disabled", "PASS",
                f"rejected ({wf_errs[0]['message'][:80]!r})")
    finally:
        ctx.client.set_global_setting("api_mode_enabled", True)
