"""A11 — api-mode agent invokes a `kind='tool'` python_scripts row.

Tests the in-process Python tool handler
(be/internal/spawner/apirun/tools_python/python.go):
  - `loadProjectPythonTools` (spawner/spawner.go) loads every
    `python_scripts` row with kind='tool' for the project and registers
    one apirun.ToolHandler per row.
  - The model calls the tool by its row name; the handler schema-
    validates input, spawns the per-project python interpreter, feeds
    JSON on stdin, captures stdout, and returns the result.
  - Each invocation inserts a `tool_dispatches` row (status='success')
    and broadcasts `ws.EventToolDispatched`.

Expected PASS:
  - agent_sessions.result == 'pass'
  - findings.tool_output contains the canonical response 'red:3'
  - exactly one tool_dispatches row for the project with
    tool_name='record_color' and status='success'
"""

from __future__ import annotations

import json

from lib import db as db_mod
from lib.runtime import (
    Ctx, PASS_STATUSES, Result,
    first_session, make_project, next_id, resolve_model, wait_for_workflow,
)


MODELS_BY_PROVIDER: dict[str, str] = {}

TOOL_CODE = """\
import sys, json
data = json.loads(sys.stdin.read() or "{}")
color = data.get("color", "")
print(json.dumps({"result": f"{color}:{len(color)}"}))
"""

INPUT_SCHEMA = json.dumps({
    "type": "object",
    "properties": {"color": {"type": "string"}},
    "required": ["color"],
    "additionalProperties": False,
})

PROMPT = """\
You are an integration-test agent running in api-mode. Call tools in
this exact order and emit no other text:
  1. `record_color` with {"color": "red"}
  2. `findings_add` with {"key": "tool_output", "value": "<the result field from step 1>"}
  3. `agent_finished` with {}
"""


def run(ctx: Ctx) -> Result:
    pid, _root = make_project(ctx)
    ctx.client.create_python_script(
        pid, name="record_color",
        kind="tool",
        code=TOOL_CODE,
        tool_description="Record a color and return its length",
        input_schema=INPUT_SCHEMA,
        timeout_sec=15,
    )

    wid = next_id(ctx, "wf")
    ctx.client.create_workflow(pid, wid, scope_type="project")
    ctx.client.create_agent_def(
        pid, wid, "main",
        model=resolve_model(ctx, MODELS_BY_PROVIDER),
        layer=0, timeout=90, prompt=PROMPT,
        tools="record_color,findings_add,agent_finished",
    )
    wfi = ctx.client.run_project_workflow(
        pid, wid, instructions="api python tool",
    )["instance_id"]
    wait_for_workflow(ctx, pid, instance_id=wfi)

    sess = first_session(db_mod.agent_sessions_for_instance(ctx.server.home, wfi))
    if sess["status"] not in PASS_STATUSES or sess["result"] != "pass":
        return ("A11 python tool dispatch", "FAIL",
                f"status/result = {sess['status']}/{sess['result']}")

    tool_output = (sess.get("findings") or {}).get("tool_output")
    if tool_output != "red:3":
        return ("A11 python tool dispatch", "FAIL",
                f"findings.tool_output = {tool_output!r}, want 'red:3' "
                "(model did not pass through the tool result)")

    dispatches = [
        d for d in db_mod.tool_dispatches(ctx.server.home, pid)
        if d.get("tool_name") == "record_color"
    ]
    if len(dispatches) != 1:
        return ("A11 python tool dispatch", "FAIL",
                f"tool_dispatches for record_color = {len(dispatches)}, want 1")
    if dispatches[0].get("status") != "success":
        return ("A11 python tool dispatch", "FAIL",
                f"dispatch status = {dispatches[0].get('status')!r}, "
                "want 'success'")
    return ("A11 python tool dispatch", "PASS",
            f"dispatch_id={dispatches[0]['id']} duration_ms="
            f"{dispatches[0].get('duration_ms')}")
