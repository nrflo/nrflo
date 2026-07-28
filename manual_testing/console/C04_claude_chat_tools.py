"""C04 — console-chat MCP tool dispatch end-to-end (claude engine).

The console engine's real value is the injected `agent mcp-external`
bridge adopting the pre-minted session (NRFLO_CONSOLE_TOKEN /
NRFLO_CONSOLE_SESSION_ID) and proxying to the server-owned console tool
catalogue. Seed one ticket, then ask the model to call the nrflo
ticket_list tool and echo the ticket id — the id can only come back
through a live tool round-trip. Console yolo is default-ON (migration
000208), so the tool call executes without a PreToolUse approval
round-trip; `approve='allow_for_session'` is passed defensively so the
wait loop still answers correctly if an approval request ever does
arrive, but this scenario does not assert one occurs.
"""

from __future__ import annotations

from lib import console as con
from lib.runtime import Ctx, Result, make_project, next_id


def run(ctx: Ctx) -> Result:
    pid, _root = make_project(ctx)
    tid = next_id(ctx, "tk")
    ctx.client.create_ticket(pid, ticket_id=tid, title="console tool probe")

    sid = con.create_chat(ctx, pid, engine="claude", model=ctx.model)
    try:
        con.send_message(
            ctx, sid,
            "Call the nrflo ticket_list tool now and reply with the exact "
            "id of every ticket it returns, nothing else.")
        reply = con.wait_assistant_reply(ctx, sid, contains=tid,
                                         approve="allow_for_session")
        con.wait_turn_idle(ctx, sid)
    finally:
        con.close_chat(ctx, sid)

    return ("C04", "PASS",
            f"ticket_list round-trip surfaced {tid} (reply {len(reply)} chars)")
