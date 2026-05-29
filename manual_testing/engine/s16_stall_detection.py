"""S16 — Spawner stall detection (running-stall path).

An agent calls a `kind='tool'` python_scripts tool whose handler blocks
far longer than the stall window. The MCP tool call is dispatched
server-side and blocks the agent's turn, so no tool/result hook events
fire while it runs — with stall_running_timeout_sec=15 the spawner trips
its stall detector (result='continue', reason 'stall_restart_running_stall')
and auto-relaunches.

A blocking MCP tool is used rather than `sleep 30` in Bash: the modern
claude CLI auto-backgrounds long shell commands (returning a
backgroundTaskId) and the model may "wait" with a non-blocking tool such
as Monitor, so a shell sleep no longer reliably produces the silence the
stall detector needs. A server-side MCP tool call cannot be backgrounded
or escaped, so the stall fires deterministically (mirrors api-mode A09).

Expected PASS:
  - At least one agent_sessions row with result_reason containing 'stall'
    within 120 seconds (the stall timer itself is 15s; the extra slack
    covers spawner relaunch and PTY contention).
"""

from __future__ import annotations

import json
import time

from lib import db as db_mod
from lib.runtime import Ctx, Result, make_project, next_id, resolve_model


# Per-provider model overrides; empty = use the runner default (e.g. haiku).
MODELS_BY_PROVIDER: dict[str, str] = {}


STALL_TIMEOUT_SEC = 15
SLOW_DELAY_SEC = 60      # >> stall window; the tool blocks long past the trip
TOOL_TIMEOUT_SEC = 120   # >> stall window so the stall fires before any tool timeout

# A tool that never returns within the stall window: sleep, then (never
# reached before the stall kills the agent) print a result.
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
You are an integration-test agent. Do EXACTLY this and nothing else, in
order:

1. Call the `slow_probe` tool exactly once with input {"q": "ping"}.
2. Only after it returns, call the `agent_finished` tool with {}.

Do NOT use Bash, Monitor, or any other tool to wait — call `slow_probe`
directly. Produce no other output.
"""


def run(ctx: Ctx) -> Result:
    pid, _root = make_project(ctx)

    # A blocking server-side tool the agent cannot background or escape.
    ctx.client.create_python_script(
        pid, name="slow_probe",
        kind="tool",
        code=TOOL_CODE,
        tool_description="Deliberately slow probe — never returns within the stall window.",
        input_schema=INPUT_SCHEMA,
        timeout_sec=TOOL_TIMEOUT_SEC,
    )

    wid = next_id(ctx, "wf")
    ctx.client.create_workflow(pid, wid, scope_type="project")
    ctx.client.create_agent_def(
        pid, wid, "main",
        # Per-agent timeout must comfortably exceed stall_*_timeout_sec, otherwise
        # the timeout-kill races the stall detector and the test ends with
        # `cancelled` instead of `stall_restart_*`.
        model=resolve_model(ctx, MODELS_BY_PROVIDER), layer=0, timeout=120, prompt=PROMPT,
        tools="slow_probe,agent_finished",
        stall_running_timeout_sec=STALL_TIMEOUT_SEC,
        stall_start_timeout_sec=STALL_TIMEOUT_SEC,
    )
    wfi = ctx.client.run_project_workflow(
        pid, wid, instructions="stall test",
    )["instance_id"]

    deadline = time.monotonic() + 120.0
    while time.monotonic() < deadline:
        sessions = db_mod.agent_sessions_for_instance(ctx.server.home, wfi)
        stalled = [s for s in sessions
                   if "stall" in (s.get("result_reason") or "")]
        if stalled:
            try:
                ctx.client.stop_project_workflow(pid, instance_id=wfi)
            except Exception:
                pass
            return ("S16 stall detection", "PASS",
                    f"stall fired: {stalled[0].get('result_reason')!r}")
        time.sleep(2)
    try:
        ctx.client.stop_project_workflow(pid, instance_id=wfi)
    except Exception:
        pass
    return ("S16 stall detection", "FAIL",
            "no stall_* result_reason within 120s")
