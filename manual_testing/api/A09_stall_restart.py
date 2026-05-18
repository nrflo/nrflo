"""A09 — api-mode stall detection on a hung HTTP tool.

Registers an HTTP `tool_definition` pointing at a deliberately slow mock
HTTP server, then asks the agent to call it. While the tool handler
hangs, no text/tool_use events fire, so `TrackMessage` in
`apirun/sink.go` falls silent and the stall detector in
`stall_restart.go` trips after `stall_running_timeout_sec` seconds.

Expected PASS:
  - within 90s, at least one agent_sessions row exists with
    `result_reason` containing 'stall'.
"""

from __future__ import annotations

import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

from lib import db as db_mod
from lib.runtime import Ctx, Result, make_project, next_id, resolve_model


MODELS_BY_PROVIDER: dict[str, str] = {}

STALL_TIMEOUT_SEC = 5
SLOW_DELAY_SEC = 60.0    # > HTTP tool default timeout, > stall window


def _start_slow_server() -> tuple[ThreadingHTTPServer, str]:
    """Spin a ThreadingHTTPServer whose POST handler sleeps long enough
    that the stall detector trips first. Returns (server, url)."""
    class Slow(BaseHTTPRequestHandler):
        def do_POST(self) -> None:  # noqa: N802
            time.sleep(SLOW_DELAY_SEC)
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(b'{"ok":true}')

        def log_message(self, *_a, **_kw) -> None:
            return

    srv = ThreadingHTTPServer(("127.0.0.1", 0), Slow)
    threading.Thread(target=srv.serve_forever, daemon=True).start()
    url = f"http://127.0.0.1:{srv.server_address[1]}/hang"
    return srv, url


PROMPT = """\
You are an integration-test agent in api-mode. Call the `slow_probe`
tool once with input `{"q": "ping"}`. After it returns, call
`agent_finished` with {}. Do not emit any other text or tool call.
"""


def run(ctx: Ctx) -> Result:
    srv, slow_url = _start_slow_server()
    try:
        pid, _root = make_project(ctx)
        wid = next_id(ctx, "wf")
        ctx.client.create_workflow(pid, wid, scope_type="project")

        # Register the slow HTTP tool scoped to this project so it does
        # not leak across scenarios.
        ctx.client._request("POST", "/api/v1/tool-definitions", body={
            "id": f"slow_probe_{pid}",
            "name": "slow_probe",
            "description": "Deliberately slow probe — never returns within stall window.",
            "input_schema": {
                "type": "object",
                "properties": {"q": {"type": "string"}},
                "required": ["q"],
                "additionalProperties": False,
            },
            "endpoint": slow_url,
            "auth_method": "none",
            "timeout_sec": 60,
            "project_id": pid,
        })

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
    finally:
        srv.shutdown()
        srv.server_close()
