"""S49 — Purge sensitive data on completion.

An external caller runs a project workflow with the `purge_on_completion` flag and
sensitive inputs (instructions, external_id/context, seed findings); the agent also
writes findings and messages. After the run finishes, the orchestrator's terminal
hook scrubs the trace to a redacted shell.

Expected PASS:
  - the workflow reaches a terminal status
  - agent_sessions rows are KEPT (audit shell) with agent_type intact,
    but prompt / system_prompt are NULL (redacted)
  - agent_messages for the session are deleted
  - session-scope findings (incl. workflow_final_result) are deleted
  - workflow_instance-scope findings (user_instructions + seed) are deleted
  - workflow_instances.external_id / external_context are cleared
"""

from __future__ import annotations

import time

from lib import db as db_mod
from lib.runtime import (
    Ctx, Result, make_project, next_id, resolve_model, wait_for_workflow,
)


MODELS_BY_PROVIDER: dict[str, str] = {}

SECRET_MARKER = "SENSITIVE_INSTRUCTIONS_S49"

PROMPT = """\
You are an integration-test agent. Do EXACTLY what is listed below and
nothing else. Perform the listed steps in order, then stop immediately.

1. Run: the `findings_add` tool (key=secret_note, value=topsecret)
2. Run: the `findings_add` tool (key=workflow_final_result, value="done")
3. Run: the `agent_finished` tool
"""

NAME = "S49 purge on completion"


def _purged(ctx: Ctx, wfi: str) -> bool:
    """Purge runs synchronously just after the completed broadcast, so it lands a
    beat after wait_for_workflow returns. Poll until every kept session is redacted."""
    sessions = db_mod.agent_sessions_for_instance(ctx.server.home, wfi)
    return bool(sessions) and all(not s.get("prompt") for s in sessions)


def run(ctx: Ctx) -> Result:
    pid, _root = make_project(ctx)
    wid = next_id(ctx, "wf")
    ctx.client.create_workflow(pid, wid, scope_type="project", purge_on_completion=True)
    ctx.client.create_agent_def(
        pid, wid, "main",
        model=resolve_model(ctx, MODELS_BY_PROVIDER), layer=0, timeout=5, prompt=PROMPT,
    )
    wfi = ctx.client.run_project_workflow(
        pid, wid,
        instructions=SECRET_MARKER,
        external_id="ext-secret-49",
        external_context="customer PII context",
        seed_findings={"customer_ssn": "123-45-6789"},
    )["instance_id"]

    wait_for_workflow(ctx, pid, instance_id=wfi)

    deadline = time.time() + 20
    while time.time() < deadline and not _purged(ctx, wfi):
        time.sleep(0.5)

    sessions = db_mod.agent_sessions_for_instance(ctx.server.home, wfi)
    inst = db_mod.workflow_instance(ctx.server.home, wfi) or {}

    if not sessions:
        return (NAME, "FAIL", "no agent_sessions row kept (expected a redacted shell)")

    fails: list[str] = []
    for s in sessions:
        if s.get("prompt"):
            fails.append(f"[prompt] not redacted: {s.get('prompt')!r}")
        if s.get("system_prompt"):
            fails.append("[system_prompt] not redacted")
        if s.get("findings"):
            fails.append(f"[session findings] not deleted: {s.get('findings')!r}")
        if not s.get("agent_type"):
            fails.append("[shell] agent_type missing on kept session")
        msgs = db_mod.agent_messages(ctx.server.home, s["id"])
        if msgs:
            fails.append(f"[messages] {len(msgs)} not deleted")

    if inst.get("findings"):
        fails.append(f"[instance findings] not deleted: {inst.get('findings')!r}")
    if inst.get("external_id"):
        fails.append(f"[external_id] not cleared: {inst.get('external_id')!r}")
    if inst.get("external_context"):
        fails.append(f"[external_context] not cleared: {inst.get('external_context')!r}")

    if fails:
        return (NAME, "FAIL", "; ".join(fails))
    return (NAME, "PASS", f"sessions_kept={len(sessions)} all redacted")
