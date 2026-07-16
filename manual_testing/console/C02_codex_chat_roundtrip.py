"""C02 — codex console-chat roundtrip over REST.

Same lifecycle as C01 on the codex app-server engine. SKIPs when the
codex binary is not on PATH (the folder itself boots under claude).
"""

from __future__ import annotations

import shutil

from lib import console as con
from lib.runtime import Ctx, Result, make_project

CODEX_MODEL = "codex_gpt54_mini_low"


def run(ctx: Ctx) -> Result:
    if not shutil.which("codex"):
        return ("C02", "SKIP", "codex binary not on PATH")

    pid, _root = make_project(ctx)
    sid = con.create_chat(ctx, pid, engine="codex", model=CODEX_MODEL)
    try:
        con.send_message(ctx, sid, "Reply with exactly the single word: pong")
        reply = con.wait_assistant_reply(ctx, sid, contains="pong")
        detail = con.wait_turn_idle(ctx, sid)
        if detail.get("engine") != "codex":
            return ("C02", "FAIL", f"detail engine = {detail.get('engine')!r}")
    finally:
        con.close_chat(ctx, sid)

    row = next((r for r in con.list_chats(ctx, pid)
                if r["session_id"] == sid), {})
    if row.get("live") or row.get("status") != "interactive_completed":
        return ("C02", "FAIL", f"closed chat row = {row}")

    return ("C02", "PASS", f"codex chat roundtrip ok (reply {len(reply)} chars)")
