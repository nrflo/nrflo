"""S41 — Workflow bundle export → import round-trips a runnable workflow.

Tests:
  - `GET /api/v1/workflows/{id}/export` returns a `WorkflowBundle` with
    the workflow, its agent_defs, layer policies, and notifications
    (be/internal/api/handlers_workflow_export_import.go).
  - `POST /api/v1/workflows/import/check` against a fresh project reports
    no conflicts.
  - `POST /api/v1/workflows/import` (action="overwrite") creates the
    workflow + agents in the new project; the imported workflow then
    runs cleanly to pass.

Expected PASS:
  - check result has empty workflow_ids and python_script_ids
  - import returns workflow_ids containing the original wid
  - the imported workflow run finishes with status ∈ PASS_STATUSES
"""

from __future__ import annotations

from lib import db as db_mod
from lib.runtime import (
    Ctx, PASS_STATUSES, Result,
    first_session, make_project, next_id, resolve_model, wait_for_workflow,
)


MODELS_BY_PROVIDER: dict[str, str] = {}

PROMPT = """\
You are an integration-test agent. Run the listed command via the Bash
tool, then stop.

1. Run: `nrflo agent finished`
"""


def run(ctx: Ctx) -> Result:
    # 1. source project: build a tiny project-scoped workflow.
    src_pid, _ = make_project(ctx)
    wid = next_id(ctx, "wf")
    ctx.client.create_workflow(src_pid, wid, scope_type="project")
    ctx.client.create_agent_def(
        src_pid, wid, "main",
        model=resolve_model(ctx, MODELS_BY_PROVIDER),
        layer=0, timeout=5, prompt=PROMPT,
    )

    # 2. export → 3. fresh target project → check → import.
    bundle = ctx.client.export_workflow(src_pid, wid)
    if not isinstance(bundle, dict) or not bundle.get("workflows"):
        return ("S41 workflow export/import", "FAIL",
                f"export bundle malformed: {bundle!r}")
    if bundle.get("version") != "1.0":
        return ("S41 workflow export/import", "FAIL",
                f"bundle.version = {bundle.get('version')!r}, want '1.0'")

    dst_pid, _ = make_project(ctx)
    conflicts = ctx.client.import_workflow_check(dst_pid, bundle)
    if conflicts.get("workflow_ids") or conflicts.get("python_script_ids"):
        return ("S41 workflow export/import", "FAIL",
                f"unexpected conflicts on fresh project: {conflicts!r}")

    result = ctx.client.import_workflow(dst_pid, bundle, action="overwrite")
    if wid not in (result.get("workflow_ids") or []):
        return ("S41 workflow export/import", "FAIL",
                f"import result missing wid={wid}: {result!r}")
    if result.get("skipped"):
        return ("S41 workflow export/import", "FAIL",
                "import returned skipped=true")

    # 4. run the imported workflow in dst_pid — confirms agent_defs +
    # layer policies were materialised correctly.
    wfi = ctx.client.run_project_workflow(
        dst_pid, wid, instructions="import roundtrip",
    )["instance_id"]
    wait_for_workflow(ctx, dst_pid, instance_id=wfi)

    sess = first_session(db_mod.agent_sessions_for_instance(ctx.server.home, wfi))
    if sess["status"] not in PASS_STATUSES or sess["result"] != "pass":
        return ("S41 workflow export/import", "FAIL",
                f"imported run status/result = {sess['status']}/{sess['result']}")
    return ("S41 workflow export/import", "PASS",
            f"src={src_pid} dst={dst_pid} wid={wid}")
