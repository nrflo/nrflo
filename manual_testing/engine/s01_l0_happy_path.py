"""S01 — L0 happy-path mega-scenario.

One project-scope, single-agent L0 workflow whose agent exercises every
small "side-effect" surface the engine has, then verifies all of them
in one pass. Folds the former s01/s03/s04/s06/s08/s11/s13/s24/s44
into a single workflow run.

Coverage checklist (each gets its own labeled sub-assertion):
  - session.findings.greeting == 'hello-2' + session result=pass
    (findings.add own-session, final-value semantics)
  - project_findings.team == 'alpha'                       (project-add)
  - agent_messages for session has category='tool'          (Bash → tool row)
  - workflow_instances.skip_tags contains 'flaky-step'      (nrflo skip)
  - REST workflow_final_result == 'all green' on workflow state
  - session.prompt contains the unique marker we passed as instructions
  - session.findings.color == 'red'                          (project env var)
  - /agent-session-logs row exists for this session with non-empty
    workflow_id and duration_sec >= 0                        (logs endpoint)
  - findings/history?key=greeting returns 2 rows ['add','add'] newest-first
    with old/new values ('hello-2', 'hello-1') / (None, 'hello-1')
"""

from __future__ import annotations

from lib import db as db_mod
from lib.runtime import (
    Ctx, PASS_STATUSES, Result,
    first_session, make_project, next_id, pick_instance, resolve_model,
    wait_for_workflow,
)


MODELS_BY_PROVIDER: dict[str, str] = {}

ENV_NAME = "MY_TEST_VAR"
ENV_VALUE = "red"
SKIP_TAG = "flaky-step"

PROMPT = """\
You are an integration-test agent. Do EXACTLY what is listed below and
nothing else. Use the Bash tool to run the listed commands in order,
then stop immediately.

1. Run: `nrflo findings add greeting hello-1`
2. Run: `nrflo findings add greeting hello-2`
3. Run: `nrflo findings project-add team alpha`
4. Run: `nrflo findings add color "$MY_TEST_VAR"`
5. Run: `nrflo findings add workflow_final_result "all green"`
6. Run: `nrflo skip flaky-step`
7. Run: `echo hello-from-bash`
8. Run: `nrflo agent finished`
"""

NAME = "S01 L0 happy-path"


def run(ctx: Ctx) -> Result:
    marker = "UNIQ_S01_" + next_id(ctx, "m")
    pid, _root = make_project(ctx)
    ctx.client.put_project_env_var(pid, ENV_NAME, ENV_VALUE)

    wid = next_id(ctx, "wf")
    ctx.client.create_workflow(
        pid, wid, scope_type="project", groups=[SKIP_TAG])
    ctx.client.create_agent_def(
        pid, wid, "main",
        model=resolve_model(ctx, MODELS_BY_PROVIDER),
        layer=0, timeout=5, prompt=PROMPT,
    )
    wfi = ctx.client.run_project_workflow(
        pid, wid, instructions=marker,
    )["instance_id"]
    state = wait_for_workflow(ctx, pid, instance_id=wfi)

    sess = first_session(db_mod.agent_sessions_for_instance(ctx.server.home, wfi))
    findings = sess.get("findings") or {}
    msgs = db_mod.agent_messages(ctx.server.home, sess["id"])
    cats = {m["category"] for m in msgs}
    inst = db_mod.workflow_instance(ctx.server.home, wfi) or {}
    skip_tags = inst.get("skip_tags")
    wf = pick_instance(state, wfi) or {}
    pf = db_mod.project_findings(ctx.server.home, pid)

    fails: list[str] = []

    if sess["status"] not in PASS_STATUSES or sess["result"] != "pass":
        fails.append(
            f"[session] status/result = {sess['status']}/{sess['result']}")

    if findings.get("greeting") != "hello-2":
        fails.append(f"[s01] findings.greeting = {findings.get('greeting')!r}")

    if pf.get("team") != "alpha":
        fails.append(f"[s03] project_findings.team = {pf.get('team')!r}")

    if "tool" not in cats:
        fails.append(f"[s04] no category='tool' message (saw {sorted(cats)})")

    if not isinstance(skip_tags, list) or SKIP_TAG not in skip_tags:
        fails.append(f"[s06] workflow_instances.skip_tags = {skip_tags!r}")

    if wf.get("workflow_final_result") != "all green":
        fails.append(
            f"[s08] REST workflow_final_result = "
            f"{wf.get('workflow_final_result')!r}")

    if marker not in (sess.get("prompt") or ""):
        fails.append("[s11] marker not found in rendered prompt")

    if findings.get("color") != ENV_VALUE:
        fails.append(
            f"[s13] findings.color = {findings.get('color')!r}, "
            f"want {ENV_VALUE!r}")

    logs_resp = ctx.client._request(
        "GET", "/api/v1/agent-session-logs?per_page=100", project=pid,
    )
    rows = (logs_resp.get("rows") or logs_resp.get("sessions")
            or logs_resp.get("items") or [])
    if not isinstance(rows, list) and isinstance(logs_resp, list):
        rows = logs_resp
    match = next((r for r in rows
                  if r.get("session_id") == sess["id"]
                  or r.get("id") == sess["id"]), None)
    if not match:
        fails.append(
            f"[s24] session {sess['id']} not in logs response "
            f"({len(rows)} rows)")
    else:
        if not match.get("workflow_id"):
            fails.append(f"[s24] workflow_id missing on logs row: {match}")
        dur = match.get("duration_sec")
        if dur is None or dur < 0:
            fails.append(f"[s24] duration_sec = {dur!r}")

    hist = ctx.client.findings_history(
        "session", sess["id"], project_id=pid, key="greeting",
    )
    items = hist.get("items") if isinstance(hist, dict) else None
    if not isinstance(items, list):
        fails.append(f"[s44] history response missing 'items': {hist!r}")
    elif len(items) != 2:
        fails.append(f"[s44] history length = {len(items)}, want 2")
    else:
        ops = [it.get("operation") for it in items]
        if ops != ["add", "add"]:
            fails.append(f"[s44] operations = {ops!r}, want ['add','add']")
        ov0 = items[0].get("old_value")
        if not isinstance(ov0, str) or "hello-1" not in ov0:
            fails.append(
                f"[s44] row[0].old_value = {ov0!r}, want to contain 'hello-1'")
        if items[1].get("old_value") is not None:
            fails.append(
                f"[s44] row[1].old_value = "
                f"{items[1].get('old_value')!r}, want None")
        vals = [it.get("new_value") for it in items]
        norm = [v.strip('"') if isinstance(v, str) else v for v in vals]
        if norm != ["hello-2", "hello-1"]:
            fails.append(
                f"[s44] new_values = {vals!r}, want ['hello-2','hello-1']")

    if fails:
        return (NAME, "FAIL", "; ".join(fails))
    return (NAME, "PASS",
            f"session={sess['id']} msgs={len(msgs)} project_findings={list(pf)}")
