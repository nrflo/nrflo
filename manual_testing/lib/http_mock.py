"""Tiny in-process webhook capture for notification-channel scenarios.

Spins a ThreadingHTTPServer on an ephemeral 127.0.0.1 port; records
every inbound POST (path, headers, parsed JSON body when possible).
Closes cleanly on context exit."""

from __future__ import annotations

import json
import threading
import time
from dataclasses import dataclass, field
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from typing import Any


@dataclass
class CapturedRequest:
    path: str
    headers: dict[str, str]
    body_raw: str
    body_json: Any = None


@dataclass
class _Captured:
    items: list[CapturedRequest] = field(default_factory=list)
    cv: threading.Condition = field(default_factory=threading.Condition)


def _make_handler(state: _Captured) -> type[BaseHTTPRequestHandler]:
    class Handler(BaseHTTPRequestHandler):
        def do_POST(self) -> None:  # noqa: N802 — stdlib API
            length = int(self.headers.get("Content-Length") or 0)
            raw = self.rfile.read(length).decode("utf-8", errors="replace")
            try:
                parsed = json.loads(raw)
            except json.JSONDecodeError:
                parsed = None
            req = CapturedRequest(
                path=self.path,
                headers={k: v for k, v in self.headers.items()},
                body_raw=raw,
                body_json=parsed,
            )
            with state.cv:
                state.items.append(req)
                state.cv.notify_all()
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(b'{"ok":true}')

        def log_message(self, *_a: Any, **_kw: Any) -> None:
            return  # silence stdlib console spam

    return Handler


class WebhookCapture:
    """Context manager. `.url` is the POST target; `.received` lists captures."""

    def __init__(self, *, path: str = "/hook") -> None:
        self._path = path
        self._state = _Captured()
        self._srv: ThreadingHTTPServer | None = None
        self._thread: threading.Thread | None = None
        self.url: str = ""

    def __enter__(self) -> "WebhookCapture":
        self._srv = ThreadingHTTPServer(("127.0.0.1", 0), _make_handler(self._state))
        port = self._srv.server_address[1]
        self.url = f"http://127.0.0.1:{port}{self._path}"
        self._thread = threading.Thread(target=self._srv.serve_forever, daemon=True)
        self._thread.start()
        return self

    def __exit__(self, *exc: Any) -> None:
        if self._srv is not None:
            self._srv.shutdown()
            self._srv.server_close()
            self._srv = None
        if self._thread is not None:
            self._thread.join(timeout=2)
            self._thread = None

    @property
    def received(self) -> list[CapturedRequest]:
        with self._state.cv:
            return list(self._state.items)

    def wait_for(self, n: int = 1, *, timeout_s: float = 15.0) -> bool:
        deadline = time.monotonic() + timeout_s
        with self._state.cv:
            while len(self._state.items) < n:
                remaining = deadline - time.monotonic()
                if remaining <= 0:
                    return False
                self._state.cv.wait(timeout=remaining)
            return True
