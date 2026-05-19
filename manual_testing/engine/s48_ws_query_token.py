"""S48 — ?token= query auth on WS for service-token bearer.

Tests the `requireAuthWith(true, ...)` query-token fallback wired into
`GET /api/v1/ws` (be/internal/api/server.go) — browsers cannot set
`Authorization` headers on WebSocket constructors, so the route accepts
`?token=<bearer>`. The token resolves through the same service-token
lookup as header bearer auth.

Expected PASS:
  - WS with ?token=<service_token> opens, can subscribe, and receives a
    `subscribed` ack (i.e. the upgrade + subscribe round-trip succeeds
    purely on query-token auth — no cookie, no Authorization header).
  - WS with ?token=<bogus> rejected before upgrade (handshake error).
"""

from __future__ import annotations

import http.cookiejar
import importlib.util

from lib.runtime import Ctx, Result, make_project
from lib.ws_client import WSClient


def run(ctx: Ctx) -> Result:
    if importlib.util.find_spec("websockets") is None:
        return ("S48 WS query token", "SKIP",
                "websockets package not installed (pip install websockets)")

    pid, _root = make_project(ctx)
    minted = ctx.client.create_service_token(pid, name=f"s48-{pid}")
    token = (minted or {}).get("token") if isinstance(minted, dict) else None
    if not isinstance(token, str) or not token:
        return ("S48 WS query token", "FAIL",
                f"mint response missing 'token': {minted!r}")

    # Happy path: query-token WS subscribes and receives the ack envelope.
    with WSClient(ctx.server.base_url, query_token=token) as ws:
        ws.subscribe(pid, since_seq=0)
        ack = ws.wait_for(
            lambda e: (e.get("type") == "ack"
                       and e.get("action") == "subscribed"
                       and e.get("project_id") == pid),
            timeout_s=10.0,
        )
        if ack is None:
            return ("S48 WS query token", "FAIL",
                    "no 'subscribed' ack on query-token WS within 10s")

    # Negative path: bogus token must fail the WS upgrade handshake.
    bogus_jar = http.cookiejar.CookieJar()  # empty
    try:
        with WSClient(ctx.server.base_url, bogus_jar,
                      query_token="not-a-valid-token") as _:
            return ("S48 WS query token", "FAIL",
                    "WS upgrade succeeded with bogus token (want 401)")
    except Exception:
        # websockets raises InvalidStatus / InvalidHandshake on 401 — we don't
        # care which class, only that the upgrade was rejected.
        pass

    return ("S48 WS query token", "PASS",
            f"?token=<service_token> upgraded + subscribed; bogus rejected")
