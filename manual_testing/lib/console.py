"""Console-chat REST helpers for the manual-testing harness.

Thin wrappers over `NrfloClient._request` for the `/api/v1/console/*`
surface (kind='console_chat' sessions). The console folder is engine-scoped
rather than provider-scoped: one scenario file per engine, each gating on
its own binary/credentials instead of branching on `ctx.provider`.
"""

from __future__ import annotations

import time
from typing import Any

from . import runtime


def create_chat(ctx: runtime.Ctx, project_id: str, *, engine: str,
                model: str | None = None) -> str:
    """POST /api/v1/console/chats → session_id."""
    body: dict[str, Any] = {"engine": engine}
    if model:
        body["model"] = model
    resp = ctx.client._request("POST", "/api/v1/console/chats",
                               body=body, project=project_id)
    sid = resp.get("session_id") if isinstance(resp, dict) else None
    if not sid:
        raise AssertionError(f"create chat returned no session_id: {resp!r}")
    return sid


def get_chat(ctx: runtime.Ctx, sid: str) -> dict:
    """GET /api/v1/console/chats/{sid} — detail snapshot (turn, live, …)."""
    return ctx.client._request("GET", f"/api/v1/console/chats/{sid}")


def list_chats(ctx: runtime.Ctx, project_id: str) -> list[dict]:
    resp = ctx.client._request("GET", "/api/v1/console/chats",
                               project=project_id)
    return resp.get("sessions", []) if isinstance(resp, dict) else []


def catalog(ctx: runtime.Ctx, project_id: str) -> dict:
    return ctx.client._request("GET", "/api/v1/console/catalog",
                               project=project_id)


def send_message(ctx: runtime.Ctx, sid: str, text: str) -> None:
    """POST one user turn; the route answers 202 with an empty body."""
    ctx.client._request("POST", f"/api/v1/console/chats/{sid}/messages",
                        body={"text": text})


def close_chat(ctx: runtime.Ctx, sid: str) -> None:
    ctx.client._request("POST", f"/api/v1/console/chats/{sid}/close")


def get_messages(ctx: runtime.Ctx, sid: str) -> list[dict]:
    resp = ctx.client._request("GET", f"/api/v1/console/chats/{sid}/messages")
    return resp.get("messages", []) if isinstance(resp, dict) else []


def wait_turn_idle(ctx: runtime.Ctx, sid: str, *,
                   require_ran: bool = False) -> dict:
    """Poll the detail snapshot until turn == 'idle' (and, when
    `require_ran`, until a 'running' state was observed first so an
    engine that never started the turn fails loudly). Returns the last
    detail payload."""
    deadline = time.monotonic() + runtime.RUN_TIMEOUT_S
    saw_running = False
    detail: dict = {}
    while time.monotonic() < deadline:
        detail = get_chat(ctx, sid)
        if not detail.get("live"):
            raise AssertionError(f"chat {sid} lost its live engine: {detail}")
        turn = detail.get("turn")
        if turn == "running":
            saw_running = True
        if turn == "idle" and (saw_running or not require_ran):
            return detail
        time.sleep(runtime.POLL_INTERVAL_S)
    raise TimeoutError(
        f"chat {sid} turn did not go idle in {runtime.RUN_TIMEOUT_S}s "
        f"(last={detail.get('turn')!r}, saw_running={saw_running})")


def reply_approval(ctx: runtime.Ctx, sid: str, approval_id: str,
                   decision: str = "allow") -> None:
    ctx.client._request(
        "POST",
        f"/api/v1/console/chats/{sid}/approvals/{approval_id}",
        body={"decision": decision})


def wait_assistant_reply(ctx: runtime.Ctx, sid: str, *,
                         contains: str | None = None,
                         approve: str | None = None) -> str:
    """Poll message history until a non-user message arrives (optionally
    containing `contains`, case-insensitive). Returns its content.

    A `system` provider_error row fails fast instead of burning the whole
    timeout. When `approve` is set ('allow' / 'allow_for_session'), any
    pending approval surfaced by the detail snapshot is answered with that
    decision — tool-calling scenarios need this or the engine blocks on a
    human decision forever."""
    deadline = time.monotonic() + runtime.RUN_TIMEOUT_S
    answered: set[str] = set()
    while time.monotonic() < deadline:
        if approve:
            detail = get_chat(ctx, sid)
            for a in detail.get("pending_approvals") or []:
                aid = a.get("approval_id")
                if aid and aid not in answered:
                    reply_approval(ctx, sid, aid, approve)
                    answered.add(aid)
        for msg in get_messages(ctx, sid):
            category = msg.get("category")
            content = msg.get("content") or ""
            if category == "system" and "provider_error" in content:
                raise AssertionError(f"chat {sid} provider error: {content[:300]}")
            if category == "user_input":
                continue
            if contains is None or contains.lower() in content.lower():
                return content
        time.sleep(runtime.POLL_INTERVAL_S)
    want = f" containing {contains!r}" if contains else ""
    raise TimeoutError(
        f"chat {sid}: no assistant message{want} in {runtime.RUN_TIMEOUT_S}s")
