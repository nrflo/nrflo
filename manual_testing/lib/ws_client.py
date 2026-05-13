"""Minimal sync WebSocket client for `/api/v1/ws` (protocol v2).

Used by scenarios that need to observe live server events. Cookie-based
auth is reused from the existing `NrfloClient._jar` so the WS upgrade
carries the same admin session as REST calls.

Adds a `websockets` runtime dep (PyPI). Install with `pip install
websockets`. Falls back to a clear error if absent.
"""

from __future__ import annotations

import http.cookiejar
import json
import time
from typing import Any, Callable


def _load_connect():
    """Import lazily so the harness loads even when websockets isn't
    installed. Only s37 (which constructs a WSClient) needs it."""
    try:
        from websockets.sync.client import connect as _ws_connect
    except ImportError as e:  # pragma: no cover
        raise RuntimeError(
            "manual_testing.lib.ws_client requires the `websockets` package: "
            "pip install websockets"
        ) from e
    return _ws_connect


def _cookie_header(jar: http.cookiejar.CookieJar) -> str:
    return "; ".join(f"{c.name}={c.value}" for c in jar)


class WSClient:
    """Open a WS connection authed with the same cookies as the REST client.

    Use as a context manager so the connection closes deterministically:

        with WSClient(base_url, jar) as ws:
            ws.subscribe(project_id, since_seq=0)
            ev = ws.wait_for(lambda e: e.get("type") == "agent.completed")
    """

    def __init__(self, base_url: str, jar: http.cookiejar.CookieJar) -> None:
        ws_url = base_url.replace("http://", "ws://", 1).replace(
            "https://", "wss://", 1) + "/api/v1/ws"
        self._url = ws_url
        self._headers = [("Cookie", _cookie_header(jar))]
        self._conn = None

    def __enter__(self) -> "WSClient":
        connect = _load_connect()
        self._conn = connect(
            self._url, additional_headers=self._headers, open_timeout=10,
        )
        return self

    def __exit__(self, *exc: Any) -> None:
        if self._conn is not None:
            try:
                self._conn.close()
            except Exception:
                pass
            self._conn = None

    def subscribe(self, project_id: str, *, ticket_id: str = "",
                  since_seq: int = 0) -> None:
        if self._conn is None:
            raise RuntimeError("WSClient not opened (use as context manager)")
        msg: dict[str, Any] = {
            "action": "subscribe",
            "project_id": project_id,
            "ticket_id": ticket_id,
            "since_seq": since_seq,
        }
        self._conn.send(json.dumps(msg))

    def wait_for(
        self,
        predicate: Callable[[dict], bool],
        *,
        timeout_s: float = 30.0,
    ) -> dict | None:
        """Return the first event the predicate accepts, or None on timeout.
        Snapshot/heartbeat envelopes pass through unfiltered — predicates
        choose what they care about."""
        if self._conn is None:
            raise RuntimeError("WSClient not opened (use as context manager)")
        deadline = time.monotonic() + timeout_s
        while time.monotonic() < deadline:
            remaining = max(0.05, deadline - time.monotonic())
            try:
                raw = self._conn.recv(timeout=remaining)
            except TimeoutError:
                continue
            except Exception:
                return None
            try:
                event = json.loads(raw)
            except json.JSONDecodeError:
                continue
            if predicate(event):
                return event
        return None
