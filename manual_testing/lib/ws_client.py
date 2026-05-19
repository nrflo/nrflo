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
import urllib.parse
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

    def __init__(
        self,
        base_url: str,
        jar: http.cookiejar.CookieJar | None = None,
        *,
        query_token: str | None = None,
    ) -> None:
        ws_url = base_url.replace("http://", "ws://", 1).replace(
            "https://", "wss://", 1) + "/api/v1/ws"
        if query_token is not None:
            ws_url += "?token=" + urllib.parse.quote(query_token, safe="")
        self._url = ws_url
        # Cookie-auth path keeps the existing header; query-token path sends
        # no Cookie/Authorization so the ?token= fallback in requireAuthWith
        # is exercised end-to-end.
        if query_token is None:
            if jar is None:
                raise ValueError("WSClient requires either jar or query_token")
            self._headers = [("Cookie", _cookie_header(jar))]
        else:
            self._headers = []
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
                  since_seq: int | None = 0) -> None:
        """Subscribe to events.

        `since_seq=0` (default) triggers an initial snapshot stream — keeps
        existing scenarios (s37) on the path they expect. Pass
        `since_seq=None` for live-only delivery (no snapshot), which avoids
        the snapshot fan-out flooding the per-client send buffer (256) and
        causing fast live broadcasts to be dropped silently — important
        for race-tight tests like s47/A13 where the event of interest is
        broadcast within milliseconds of run start.
        """
        if self._conn is None:
            raise RuntimeError("WSClient not opened (use as context manager)")
        msg: dict[str, Any] = {
            "action": "subscribe",
            "project_id": project_id,
            "ticket_id": ticket_id,
        }
        if since_seq is not None:
            msg["since_seq"] = since_seq
        self._conn.send(json.dumps(msg))

    def wait_for(
        self,
        predicate: Callable[[dict], bool],
        *,
        timeout_s: float = 30.0,
    ) -> dict | None:
        """Return the first event the predicate accepts, or None on timeout.
        Snapshot/heartbeat envelopes pass through unfiltered — predicates
        choose what they care about.

        WritePump in be/internal/ws/client.go batches all queued messages
        into a single websocket frame separated by `\\n` whenever multiple
        broadcasts arrive between WritePump iterations (fast-broadcast paths
        like rate-limit + agent_completed). Split the frame on newlines and
        decode each JSON object independently — naive `json.loads(raw)` on
        a batched frame raises and the trailing events would be silently
        discarded."""
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
            for line in raw.split("\n"):
                line = line.strip()
                if not line:
                    continue
                try:
                    event = json.loads(line)
                except json.JSONDecodeError:
                    continue
                if predicate(event):
                    return event
        return None
