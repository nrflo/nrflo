"""A09 — api-mode stall detection on a hung python tool.

Registers a `kind='tool'` python_scripts row whose handler sleeps far
longer than the stall window, then asks the agent to call it once. While
the tool blocks, no text/tool_use events fire, so `TrackMessage` in
`apirun/sink.go` falls silent and the stall detector in
`stall_restart.go` trips after `stall_running_timeout_sec` seconds.

The python tool's own `timeout_sec` is set well above the stall window so
the stall fires first — a tool timeout would emit a tool_result and reset
the heartbeat, masking the stall.

(The original variant drove this via the HTTP `tool_definitions` feature,
removed in commit d4258385 / migration 000134 — python tools replaced it.)

Expected PASS:
  - within 90s, at least one agent_sessions row exists with
    `result_reason` containing 'stall'.
"""

from __future__ import annotations

import json
import time

from lib import db as db_mod
from lib.runtime import Ctx, Result, make_project, next_id, resolve_model


MODELS_BY_PROVIDER: dict[str, str] = {}

STALL_TIMEOUT_SEC = 5
SLOW_DELAY_SEC = 60      # >> stall window; the tool blocks long past the trip
TOOL_TIMEOUT_SEC = 120   # >> stall window so the stall fires before any tool timeout

# A tool that never returns within the stall window: sleep, then (never
# reached before the stall kills the runner) print a result. The printed
# payload is irrelevant — the stall detector trips long before it returns.
TOOL_CODE = (
    "import time\n"
    f"time.sleep({SLOW_DELAY_SEC})\n"
    'print("{}")\n'
)

INPUT_SCHEMA = json.dumps({
    "type": "object",
    "properties": {"q": {"type": "string"}},
    "required": ["q"],
    "additionalProperties": False,
})

PROMPT = """\
You are an integration-test agent in api-mode. Call the `slow_probe`
tool once with input `{"q": "ping"}`. After it returns, call
`agent_finished` with {}. Do not emit any other text or tool call.
"""


def run(ctx: Ctx) -> Result:
    pid, _root = make_project(ctx)

    # Register the slow tool scoped to this project so it does not leak
    # across scenarios.
    ctx.client.create_python_script(
        pid, name="slow_probe",
        kind="tool",
        code=TOOL_CODE,
        tool_description="Deliberately slow probe — never returns within stall window.",
        input_schema=INPUT_SCHEMA,
        timeout_sec=TOOL_TIMEOUT_SEC,
    )

    wid = next_id(ctx, "wf")
    ctx.client.create_workflow(pid, wid, scope_type="project")
    ctx.client.create_agent_def(
        pid, wid, "main",
        model=resolve_model(ctx, MODELS_BY_PROVIDER),
        # Per-agent timeout must comfortably exceed stall_*_timeout_sec
        # (mirrors engine/s16 — otherwise the timeout-kill races the
        # stall detector).
        layer=0, timeout=180, prompt=PROMPT,
        tools="slow_probe,agent_finished",
        stall_running_timeout_sec=STALL_TIMEOUT_SEC,
        stall_start_timeout_sec=STALL_TIMEOUT_SEC,
    )
    wfi = ctx.client.run_project_workflow(
        pid, wid, instructions="api-mode stall",
    )["instance_id"]

    deadline = time.monotonic() + 90.0
    while time.monotonic() < deadline:
        sessions = db_mod.agent_sessions_for_instance(ctx.server.home, wfi)
        stalled = [s for s in sessions
                   if "stall" in (s.get("result_reason") or "")]
        if stalled:
            try:
                ctx.client.stop_project_workflow(pid, instance_id=wfi)
            except Exception:
                pass
            return ("A09 stall restart", "PASS",
                    f"stall fired: {stalled[0].get('result_reason')!r}")
        time.sleep(2)
    try:
        ctx.client.stop_project_workflow(pid, instance_id=wfi)
    except Exception:
        pass
    return ("A09 stall restart", "FAIL",
            "no stall_* result_reason within 90s")
