"""A10 — api-mode auth error classification.

Sets a bogus `ANTHROPIC_OAUTH_TOKEN` as a per-project env var. The
spawner's `ResolveAPIKey` returns per-project env (step 2) before the
server env fallback, so the spawn succeeds (key resolution passes the
emptiness check) but the first Anthropic streaming request fails with
401/403. `classifyProviderError` maps that to a system message
prefixed with `auth_error:`, recorded both as an `errors` row and a
`system` agent_message.

Expected PASS:
  - agent_sessions.result == 'fail' with result_reason == 'api_error'
  - errors_for_project contains a row whose message starts with
    'auth_error:'.
"""

from __future__ import annotations

from lib import db as db_mod
from lib.runtime import (
    Ctx, Result, first_session, make_project, next_id, resolve_model, wait_for_workflow,
)


MODELS_BY_PROVIDER: dict[str, str] = {}

PROMPT = """\
You are an integration-test agent in api-mode. Call `agent_finished`
with {} and stop.
"""


def run(ctx: Ctx) -> Result:
    pid, _root = make_project(ctx)
    ctx.client.put_project_env_var(
        pid, "ANTHROPIC_OAUTH_TOKEN", "sk-ant-oat01-INVALID-FOR-AUTH-ERROR-TEST",
    )

    wid = next_id(ctx, "wf")
    ctx.client.create_workflow(pid, wid, scope_type="project")
    ctx.client.create_agent_def(
        pid, wid, "main",
        model=resolve_model(ctx, MODELS_BY_PROVIDER),
        layer=0, timeout=60, prompt=PROMPT,
        tools="agent_finished",
    )
    wfi = ctx.client.run_project_workflow(
        pid, wid, instructions="api-mode auth error",
    )["instance_id"]
    try:
        wait_for_workflow(ctx, pid, instance_id=wfi)
    except TimeoutError:
        return ("A10 auth error", "FAIL", "workflow did not terminate")

    sess = first_session(db_mod.agent_sessions_for_instance(ctx.server.home, wfi))
    if sess.get("result") != "fail":
        return ("A10 auth error", "FAIL",
                f"result = {sess.get('result')!r}, want 'fail'")
    if sess.get("result_reason") != "api_error":
        return ("A10 auth error", "FAIL",
                f"result_reason = {sess.get('result_reason')!r}, "
                "want 'api_error' (mapFinalStatus FAIL -> api_error)")
    errs = db_mod.errors_for_project(ctx.server.home, pid)
    auth = [e for e in errs if "auth_error" in (e.get("message") or "")]
    if not auth:
        return ("A10 auth error", "FAIL",
                f"no errors row with auth_error prefix "
                f"(saw {[e.get('message') for e in errs]})")
    return ("A10 auth error", "PASS",
            f"auth_error classified: {auth[0]['message'][:80]!r}")
