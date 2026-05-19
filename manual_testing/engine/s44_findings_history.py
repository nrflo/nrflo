"""S44 — Findings audit history records insert + update operations.

Tests:
  - Two `nrflo findings add greeting <v>` calls on the same key create
    one current findings row and two history rows (an insert followed
    by an update) per the structured findings table introduced in
    migration 000110.
  - `GET /api/v1/findings/history?scope=session&scope_id=<sid>&key=greeting`
    returns those two rows oldest-to-newest via
    be/internal/api/handlers_findings_history.go.

Expected PASS:
  - history.items length is exactly 2
  - both rows are operation='add' (the canonical upsert op recorded by
    `findings.add`; insert vs update lives in old_value)
  - rows are newest-first per ListHistory (created_at DESC), so the
    first row carries old_value="hello-1" / new_value="hello-2" and
    the second row carries old_value=None / new_value="hello-1"
"""

from __future__ import annotations

from lib import db as db_mod
from lib.runtime import (
    Ctx, PASS_STATUSES, Result,
    first_session, make_project, next_id, resolve_model, wait_for_workflow,
)


MODELS_BY_PROVIDER: dict[str, str] = {}

PROMPT = """\
You are an integration-test agent. Use the Bash tool to run each
command in order, then stop.

1. Run: `nrflo findings add greeting hello-1`
2. Run: `nrflo findings add greeting hello-2`
3. Run: `nrflo agent finished`
"""


def _values(items: list[dict]) -> list[str | None]:
    return [it.get("new_value") for it in items]


def run(ctx: Ctx) -> Result:
    pid, _root = make_project(ctx)
    wid = next_id(ctx, "wf")
    ctx.client.create_workflow(pid, wid, scope_type="project")
    ctx.client.create_agent_def(
        pid, wid, "main",
        model=resolve_model(ctx, MODELS_BY_PROVIDER),
        layer=0, timeout=5, prompt=PROMPT,
    )
    wfi = ctx.client.run_project_workflow(
        pid, wid, instructions="findings history",
    )["instance_id"]
    wait_for_workflow(ctx, pid, instance_id=wfi)

    sess = first_session(db_mod.agent_sessions_for_instance(ctx.server.home, wfi))
    if sess["status"] not in PASS_STATUSES or sess["result"] != "pass":
        return ("S44 findings history", "FAIL",
                f"status/result = {sess['status']}/{sess['result']}")

    body = ctx.client.findings_history(
        "session", sess["id"], project_id=pid, key="greeting",
    )
    items = body.get("items") if isinstance(body, dict) else None
    if not isinstance(items, list):
        return ("S44 findings history", "FAIL",
                f"history response missing 'items': {body!r}")
    if len(items) != 2:
        return ("S44 findings history", "FAIL",
                f"history length = {len(items)}, want 2; items={items!r}")

    ops = [it.get("operation") for it in items]
    if ops != ["add", "add"]:
        return ("S44 findings history", "FAIL",
                f"operations = {ops!r}, want ['add', 'add']")
    # Newest-first ordering: first row is the update (old="hello-1"),
    # second row is the insert (old=None).
    ov0 = items[0].get("old_value")
    if not isinstance(ov0, str) or "hello-1" not in ov0:
        return ("S44 findings history", "FAIL",
                f"row[0].old_value = {ov0!r}, want to contain 'hello-1'")
    if items[1].get("old_value") is not None:
        return ("S44 findings history", "FAIL",
                f"row[1].old_value = {items[1].get('old_value')!r}, want None")

    # Values are JSON-encoded in the history table (json.Marshal of the
    # normalized string), so both quoted and unquoted are acceptable.
    vals = _values(items)
    if [v.strip('"') if isinstance(v, str) else v for v in vals] != ["hello-2", "hello-1"]:
        return ("S44 findings history", "FAIL",
                f"new_values = {vals!r}, want ['hello-2', 'hello-1'] (newest-first)")
    return ("S44 findings history", "PASS",
            f"session={sess['id']} rows={len(items)}")
