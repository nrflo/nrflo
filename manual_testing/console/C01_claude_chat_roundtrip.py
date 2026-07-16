"""C01 — claude console-chat roundtrip over REST.

Creates a project, starts a kind='console_chat' claude engine
(POST /api/v1/console/chats), verifies catalog + list visibility, sends
one user turn, waits for the assistant reply to persist and the turn to
go idle, then closes the chat and asserts the row went terminal with no
live engine left behind.
"""

from __future__ import annotations

from lib import console as con
from lib.runtime import Ctx, Result, make_project


def run(ctx: Ctx) -> Result:
    pid, _root = make_project(ctx)

    cat = con.catalog(ctx, pid)
    engines = {e.get("id"): e for e in cat.get("engines", [])}
    if "claude" not in engines or "codex" not in engines or "api" not in engines:
        return ("C01", "FAIL", f"catalog engines incomplete: {sorted(engines)}")
    if not engines["claude"].get("enabled"):
        return ("C01", "SKIP",
                engines["claude"].get("disabled_reason") or "claude engine disabled")

    sid = con.create_chat(ctx, pid, engine="claude", model=ctx.model)
    try:
        rows = {r["session_id"]: r for r in con.list_chats(ctx, pid)}
        if sid not in rows or not rows[sid].get("live"):
            return ("C01", "FAIL", f"new chat not listed live: {rows.get(sid)}")

        con.send_message(ctx, sid, "Reply with exactly the single word: pong")
        reply = con.wait_assistant_reply(ctx, sid, contains="pong")
        detail = con.wait_turn_idle(ctx, sid)
        if detail.get("engine") != "claude":
            return ("C01", "FAIL", f"detail engine = {detail.get('engine')!r}")

        resumable = {s["session_id"] for s in con.catalog(ctx, pid).get("sessions", [])}
        if sid not in resumable:
            return ("C01", "FAIL", "live chat missing from catalog sessions")
    finally:
        con.close_chat(ctx, sid)

    after = {r["session_id"]: r for r in con.list_chats(ctx, pid)}
    row = after.get(sid) or {}
    if row.get("live") or row.get("status") != "interactive_completed":
        return ("C01", "FAIL", f"closed chat row = {row}")

    return ("C01", "PASS",
            f"claude chat roundtrip ok (reply {len(reply)} chars, closed clean)")
