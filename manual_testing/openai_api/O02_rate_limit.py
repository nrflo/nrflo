"""O02 — OpenAI api-mode rate-limit detection broadcasts agent.rate_limited.

OpenAI counterpart of the anthropic `api/A13_rate_limit.py`. Exercises the
provider-agnostic rate-limit dance through the OpenAI Responses provider.

Strategy: spin up a private nrflo_server with `OPENAI_BASE_URL` pointed at a
local mock HTTP server that returns `HTTP/1.1 429` with `Retry-After: 0` to
every POST. The openai-go SDK retries twice with the zero delay
(internal/requestconfig: shouldRetry on 429, retryDelay honours Retry-After),
then surfaces the 429 as `*openai.Error`. `classifyProviderError`
(be/internal/spawner/apirun/errors.go) maps `*openai.Error{StatusCode:429}` to
`RATE_LIMITED`/`RetryClassRateLimit`; the api backend goroutine
(be/internal/spawner/backend.go) performs the same broadcast/register-stop/
UpdateRateLimitUntil/wait dance as the anthropic api lane and the
cli_interactive lane (`s47`).

The scenario uses its own server so the bogus base URL never leaks into the
other O*/reused-A* scenarios. The injected OPENAI_API_KEY only has to be
non-empty (Resolve rejects empties); the mock returns 429 regardless of it, so
no real OpenAI quota is consumed.

Expected PASS:
  - WS receives `agent.rate_limited` with retry_count=1
  - agent_sessions.rate_limit_retry_count >= 1
  - agent_sessions.effective_mode == 'api'
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
from lib.credentials import probe_openai_key
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
                    "error": {
                        "message": "mock 429 from manual-test harness",
                        "type": "rate_limit_exceeded",
                        "code": "rate_limit_exceeded",
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
        return f"http://{host}:{port}/v1/"

    def stop(self) -> None:
        self._srv.shutdown()
        self._srv.server_close()


PROMPT = """\
You are an integration-test agent in api-mode. Call `agent_finished`
with no arguments. Do not emit any other text.
"""


def run(ctx: Ctx) -> Result:
    if importlib.util.find_spec("websockets") is None:
        return ("O02 openai rate-limit", "SKIP",
                "websockets package not installed (pip install websockets)")

    key, reason = probe_openai_key()
    if not key:
        return ("O02 openai rate-limit", "SKIP", reason)

    from lib.ws_client import WSClient

    mock = _Mock429()
    base_url = mock.start()
    sub = start_server(
        cli_label="openai-O02-rl",
        extra_env={
            "OPENAI_API_KEY": key,
            # Redirect the SDK to our local 429-only mock; the OpenAI Go SDK
            # reads OPENAI_BASE_URL on client init (provider/openai/openai.go).
            "OPENAI_BASE_URL": base_url,
        },
    )
    try:
        client = NrfloClient(sub.base_url)
        client.login()
        client.default_execution_mode = "api"
        client.set_global_setting("api_mode_enabled", True)

        pid = next_id(ctx, "p-o02")
        root = sub.home / "projects" / pid
        root.mkdir(parents=True, exist_ok=True)
        (root / ".gitkeep").write_text("")
        client.create_project(pid, root_path=str(root))
        wid = next_id(ctx, "wf-o02")
        client.create_workflow(pid, wid, scope_type="project")
        client.create_agent_def(
            pid, wid, "main",
            model="gpt54_low", layer=0, timeout=60, prompt=PROMPT,
            tools="agent_finished",
        )

        with WSClient(sub.base_url, client._jar) as ws:
            ws.subscribe(pid, since_seq=None)
            # Wait for the subscribed-ack before kicking the workflow off — the
            # api lane broadcasts agent.rate_limited within milliseconds
            # (localhost 429 mock), so an unacked subscribe would miss it.
            ack = ws.wait_for(
                lambda e: (e.get("type") == "ack"
                           and e.get("action") == "subscribed"
                           and e.get("project_id") == pid),
                timeout_s=10.0,
            )
            if ack is None:
                return ("O02 openai rate-limit", "FAIL",
                        "WS subscribe ack not received within 10s")
            wfi = client.run_project_workflow(
                pid, wid, instructions="o02 openai rate-limit",
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
            return ("O02 openai rate-limit", "FAIL",
                    f"no agent.rate_limited WS event within 30s "
                    f"(mock_hits={mock.hits})")
        data = ev.get("data") or {}
        if data.get("retry_count") != 1:
            return ("O02 openai rate-limit", "FAIL",
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
            return ("O02 openai rate-limit", "FAIL",
                    f"rate_limit_retry_count did not reach 1 within 5s "
                    f"(mock_hits={mock.hits})")
        if mock.hits == 0:
            return ("O02 openai rate-limit", "FAIL",
                    "SDK never hit the mock — OPENAI_BASE_URL not honoured?")
        if (sess.get("effective_mode") or "") != "api":
            return ("O02 openai rate-limit", "FAIL",
                    f"effective_mode = {sess.get('effective_mode')!r}, want 'api'")

        return ("O02 openai rate-limit", "PASS",
                f"ws=agent.rate_limited retry_count=1 "
                f"db.retry={sess['rate_limit_retry_count']} mock_hits={mock.hits}")
    finally:
        mock.stop()
        sub.stop(keep_dir=True)
