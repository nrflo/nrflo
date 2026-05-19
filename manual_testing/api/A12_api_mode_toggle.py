"""A12 — api_mode_enabled runtime toggle takes effect without restart.

`api_mode_enabled` (be/internal/service/global_settings.go) is read
freshly on every spawn via prepareSpawn (spawner/spawner.go:971).
This scenario flips the flag off, attempts a run (rejected), then
flips it on and runs the same agent_def again (passes) — proving the
toggle is a runtime gate, not a startup-time decision.

The api/ runner sets api_mode_enabled=true once at startup; A05 already
covers the rejection path on disabled. Here we also verify the
re-enable path so the toggle round-trip is exercised end-to-end.

Expected PASS:
  - run #1 (flag off): workflow_instance.status == 'failed', 0 sessions
  - run #2 (flag on): result == 'pass' against the same agent_def
"""

from __future__ import annotations

from lib import db as db_mod
from lib.runtime import (
    Ctx, PASS_STATUSES, Result, make_project, next_id,
    resolve_model, wait_for_workflow,
)


MODELS_BY_PROVIDER: dict[str, str] = {}

PROMPT = """\
You are an integration-test agent running in api-mode. Call
`agent_finished` with {} and stop.
"""


def run(ctx: Ctx) -> Result:
    # 1. Build project + workflow + agent_def while api_mode is still
    #    enabled (agent_def creation is itself gated by the flag).
    pid, _root = make_project(ctx)
    wid = next_id(ctx, "wf")
    ctx.client.create_workflow(pid, wid, scope_type="project")
    ctx.client.create_agent_def(
        pid, wid, "main",
        model=resolve_model(ctx, MODELS_BY_PROVIDER),
        layer=0, timeout=60, prompt=PROMPT,
        tools="agent_finished",
    )

    ctx.client.set_global_setting("api_mode_enabled", False)
    try:
        # 2. First run with api_mode disabled — prepareSpawn rejects
        #    before any agent_sessions row is created.
        wfi_off = ctx.client.run_project_workflow(
            pid, wid, instructions="toggle: off",
        )["instance_id"]
        try:
            wait_for_workflow(ctx, pid, instance_id=wfi_off)
        except TimeoutError:
            return ("A12 api_mode toggle", "FAIL",
                    "off-run did not terminate within RUN_TIMEOUT_S")
        inst = db_mod.workflow_instance(ctx.server.home, wfi_off)
        if not inst or inst.get("status") != "failed":
            return ("A12 api_mode toggle", "FAIL",
                    f"off-run instance.status = {(inst or {}).get('status')!r}, "
                    "want 'failed'")
        sess_off = db_mod.agent_sessions_for_instance(ctx.server.home, wfi_off)
        if sess_off:
            return ("A12 api_mode toggle", "FAIL",
                    f"off-run unexpectedly created {len(sess_off)} sessions")

        # 3. Re-enable and re-run; same agent_def now succeeds.
        ctx.client.set_global_setting("api_mode_enabled", True)
        wfi_on = ctx.client.run_project_workflow(
            pid, wid, instructions="toggle: on",
        )["instance_id"]
        wait_for_workflow(ctx, pid, instance_id=wfi_on)
        sessions_on = db_mod.agent_sessions_for_instance(ctx.server.home, wfi_on)
        if not sessions_on:
            return ("A12 api_mode toggle", "FAIL",
                    "on-run produced zero agent_sessions")
        sess = sessions_on[0]
        if sess["status"] not in PASS_STATUSES or sess["result"] != "pass":
            return ("A12 api_mode toggle", "FAIL",
                    f"on-run status/result = {sess['status']}/{sess['result']}")
        return ("A12 api_mode toggle", "PASS",
                f"off→failed, on→pass (session={sess['id']})")
    finally:
        # Leave the runner's invariant intact for subsequent scenarios.
        ctx.client.set_global_setting("api_mode_enabled", True)
