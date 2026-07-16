"""C03 — direct-API console-chat roundtrip (apiConsoleEngine).

Flips api_mode_enabled on, starts an engine='api' chat (in-process
apirun.Conversation — no CLI, no PTY), and runs the same
message → reply → idle → close lifecycle as C01. SKIPs when no Anthropic
OAuth token was reachable at server boot (test.py injects it via
run_all's extra_env when lib/credentials can resolve one).
"""

from __future__ import annotations

from lib import console as con
from lib.credentials import probe_oauth_token
from lib.runtime import Ctx, Result, make_project

API_MODEL = "haiku"  # api_models registry id (distinct namespace from cli_models)


def run(ctx: Ctx) -> Result:
    tok, reason = probe_oauth_token()
    if not tok:
        return ("C03", "SKIP", reason)

    ctx.client.set_global_setting("api_mode_enabled", True)
    pid, _root = make_project(ctx)

    api_opt = next((e for e in con.catalog(ctx, pid).get("engines", [])
                    if e.get("id") == "api"), {})
    if not api_opt.get("enabled"):
        return ("C03", "FAIL",
                f"api engine disabled after toggle: {api_opt.get('disabled_reason')!r}")

    sid = con.create_chat(ctx, pid, engine="api", model=API_MODEL)
    try:
        con.send_message(ctx, sid, "Reply with exactly the single word: pong")
        reply = con.wait_assistant_reply(ctx, sid, contains="pong")
        detail = con.wait_turn_idle(ctx, sid)
        if detail.get("engine") != "api":
            return ("C03", "FAIL", f"detail engine = {detail.get('engine')!r}")
    finally:
        con.close_chat(ctx, sid)

    row = next((r for r in con.list_chats(ctx, pid)
                if r["session_id"] == sid), {})
    if row.get("live") or row.get("status") != "interactive_completed":
        return ("C03", "FAIL", f"closed chat row = {row}")

    return ("C03", "PASS", f"api chat roundtrip ok (reply {len(reply)} chars)")
