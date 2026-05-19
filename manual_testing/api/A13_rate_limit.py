"""A13 — api-mode rate-limit detection broadcasts agent.rate_limited.

Tests the apirun rate-limit path
(be/internal/spawner/apirun/errors.go: `classifyProviderError` returning
`RetryClassRateLimit` on status 429, and be/internal/spawner/backend.go's
api goroutine performing the same broadcast/register-stop/wait dance as
the cli_interactive lane).

Strategy: spin up a private nrflo_server with `ANTHROPIC_BASE_URL`
pointed at a local mock HTTP server that returns `HTTP/1.1 429` with
`Retry-After: 0` to every POST. The Anthropic Go SDK retries twice with
the zero delay, then surfaces the 429 as `*sdk.Error`. The api backend
flips `finalStatus=RATE_LIMITED`, broadcasts `agent.rate_limited`, and
persists `rate_limit_retry_count` on the session row — identical to the
cli_interactive lane (`s47`).

The scenario uses its own server so other A* scenarios are unaffected by
the bogus base URL.

Expected PASS:
  - WS receives `agent.rate_limited` with retry_count=1
  - agent_sessions.rate_limit_retry_count >= 1
  - mock server received at least one POST (proves SDK actually hit it)
"""

from __future__ import annotations

import importlib.util
import json
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

from lib import db as db_mod
from lib.api import NrfloClient
from lib.credentials import probe_oauth_token
from lib.runtime import Ctx, Result, next_id
from lib.server import start_server


class _Mock429:
    def __init__(self) -> None:
        self.hits = 0
        self._lock = threading.Lock()
        outer = self

        class H(BaseHTTPRequestHandler):
            def do_POST(self) -> None:  # noqa: N802
                with outer._lock:
                    outer.hits += 1
                length = int(self.headers.get("Content-Length") or 0)
                if length:
                    self.rfile.read(length)
                body = json.dumps({
                    "type": "error",
                    "error": {
                        "type": "rate_limit_error",
                        "message": "mock 429 from manual-test harness",
                    },
                }).encode("utf-8")
                self.send_response(429)
                self.send_header("Content-Type", "application/json")
                # 0 → SDK retries immediately, total elapsed ~ a few ms.
                self.send_header("Retry-After", "0")
                self.send_header("Content-Length", str(len(body)))
                self.end_headers()
                self.wfile.write(body)

            def log_message(self, *_a, **_k) -> None:  # silence stderr
                return

        self._srv = ThreadingHTTPServer(("127.0.0.1", 0), H)
        self._thr = threading.Thread(target=self._srv.serve_forever, daemon=True)

    def start(self) -> str:
        self._thr.start()
        host, port = self._srv.server_address
        return f"http://{host}:{port}/"

    def stop(self) -> None:
        self._srv.shutdown()
        self._srv.server_close()


PROMPT = """\
You are an integration-test agent in api-mode. Call `agent_finished`
with no arguments. Do not emit any other text.
"""


def run(ctx: Ctx) -> Result:
    if importlib.util.find_spec("websockets") is None:
        return ("A13 api rate-limit", "SKIP",
                "websockets package not installed (pip install websockets)")

    tok, reason = probe_oauth_token()
    if not tok:
        return ("A13 api rate-limit", "SKIP", reason)

    from lib.ws_client import WSClient

    mock = _Mock429()
    base_url = mock.start()
    sub = start_server(
        cli_label="api-A13-rl",
        extra_env={
            "ANTHROPIC_OAUTH_TOKEN": tok,
            # Redirect the SDK to our local 429-only mock; the Go SDK
            # reads ANTHROPIC_BASE_URL on client init (sdk/client.go:34).
            "ANTHROPIC_BASE_URL": base_url,
        },
    )
    try:
        client = NrfloClient(sub.base_url)
        client.login()
        client.default_execution_mode = "api"
        client.set_global_setting("api_mode_enabled", True)

        pid = next_id(ctx, "p-a13")
        root = sub.home / "projects" / pid
        root.mkdir(parents=True, exist_ok=True)
        (root / ".gitkeep").write_text("")
        client.create_project(pid, root_path=str(root))
        wid = next_id(ctx, "wf-a13")
        client.create_workflow(pid, wid, scope_type="project")
        client.create_agent_def(
            pid, wid, "main",
            model="haiku", layer=0, timeout=60, prompt=PROMPT,
            tools="agent_finished",
        )

        with WSClient(sub.base_url, client._jar) as ws:
            ws.subscribe(pid, since_seq=None)
            # Wait for the subscribed-ack before kicking the workflow off —
            # the api lane broadcasts agent.rate_limited within milliseconds
            # (localhost 429 mock), so an unacked subscribe would miss it.
            ack = ws.wait_for(
                lambda e: (e.get("type") == "ack"
                           and e.get("action") == "subscribed"
                           and e.get("project_id") == pid),
                timeout_s=10.0,
            )
            if ack is None:
                return ("A13 api rate-limit", "FAIL",
                        "WS subscribe ack not received within 10s")
            wfi = client.run_project_workflow(
                pid, wid, instructions="a13 api rate-limit",
            )["instance_id"]

            ev = ws.wait_for(
                lambda e: (e.get("type") == "agent.rate_limited"
                           and e.get("project_id") == pid),
                timeout_s=30.0,
            )

        try:
            client.stop_project_workflow(pid, instance_id=wfi)
        except Exception:
            pass

        if ev is None:
            return ("A13 api rate-limit", "FAIL",
                    f"no agent.rate_limited WS event within 30s "
                    f"(mock_hits={mock.hits})")
        data = ev.get("data") or {}
        if data.get("retry_count") != 1:
            return ("A13 api rate-limit", "FAIL",
                    f"retry_count = {data.get('retry_count')!r}, want 1")

        # Wait briefly for the DB UpdateRateLimitUntil to flush.
        deadline = time.monotonic() + 5.0
        sess = None
        while time.monotonic() < deadline:
            sessions = db_mod.agent_sessions_for_instance(sub.home, wfi)
            if sessions and (sessions[0].get("rate_limit_retry_count") or 0) >= 1:
                sess = sessions[0]
                break
            time.sleep(0.1)
        if sess is None:
            return ("A13 api rate-limit", "FAIL",
                    f"rate_limit_retry_count did not reach 1 within 5s "
                    f"(mock_hits={mock.hits})")
        if mock.hits == 0:
            return ("A13 api rate-limit", "FAIL",
                    "SDK never hit the mock — ANTHROPIC_BASE_URL not honoured?")
        if (sess.get("effective_mode") or "") != "api":
            return ("A13 api rate-limit", "FAIL",
                    f"effective_mode = {sess.get('effective_mode')!r}, want 'api'")

        return ("A13 api rate-limit", "PASS",
                f"ws=agent.rate_limited retry_count=1 "
                f"db.retry={sess['rate_limit_retry_count']} mock_hits={mock.hits}")
    finally:
        mock.stop()
        sub.stop(keep_dir=True)
