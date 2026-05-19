"""S42 — Service token bearer auth is scoped to a single project.

Tests the long-lived service token path in `requireAuth`
(be/internal/api/auth_middleware.go): a token minted for project A
satisfies authenticated routes when accompanied by X-Project=A, but
returns 403 "service token project mismatch" when used against project B.

Expected PASS:
  - POST /api/v1/service-tokens returns a one-time plaintext token
  - GET /api/v1/projects/A/workflow with Bearer + X-Project=A → 200
  - GET /api/v1/projects/B/workflow with Bearer + X-Project=B → 403
"""

from __future__ import annotations

from lib.runtime import Ctx, Result, make_project


def run(ctx: Ctx) -> Result:
    pid_a, _ = make_project(ctx)
    pid_b, _ = make_project(ctx)

    minted = ctx.client.create_service_token(pid_a, name=f"s42-{pid_a}")
    token = minted.get("token") if isinstance(minted, dict) else None
    if not isinstance(token, str) or not token:
        return ("S42 service token scope", "FAIL",
                f"mint response missing 'token': {minted!r}")
    rec = (minted or {}).get("record") or {}
    if rec.get("project_id") != pid_a:
        return ("S42 service token scope", "FAIL",
                f"record.project_id = {rec.get('project_id')!r}, want {pid_a!r}")

    # Bearer + matching X-Project → 200.
    status_ok, _ = ctx.client.bearer_get(
        f"/api/v1/projects/{pid_a}/workflow",
        token=token, project=pid_a, expect_status=200,
    )
    if status_ok != 200:
        return ("S42 service token scope", "FAIL",
                f"matching project: status = {status_ok}, want 200")

    # Bearer + mismatched X-Project → 403.
    status_bad, body_bad = ctx.client.bearer_get(
        f"/api/v1/projects/{pid_b}/workflow",
        token=token, project=pid_b, expect_status=403,
    )
    if status_bad != 403:
        return ("S42 service token scope", "FAIL",
                f"mismatched project: status = {status_bad}, want 403; "
                f"body={body_bad!r}")

    return ("S42 service token scope", "PASS",
            f"token={token[:8]}… scope_ok={pid_a} scope_block={pid_b}")
