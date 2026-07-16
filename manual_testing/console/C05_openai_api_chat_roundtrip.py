"""C05 — direct-API console-chat roundtrip on an OpenAI model.

Same lifecycle as C03, but the engine='api' chat resolves an
unified model row with provider='openai', so apirun talks to the OpenAI
Responses endpoint instead of Anthropic. SKIPs when no OpenAI key was
reachable at server boot (test.py injects OPENAI_API_KEY via run_all's
extra_env when lib/credentials can resolve one).
"""

from __future__ import annotations

from lib import console as con
from lib.credentials import probe_openai_key
from lib.runtime import Ctx, Result, make_project

OPENAI_MODEL = "gpt-5.4"


def run(ctx: Ctx) -> Result:
    key, reason = probe_openai_key()
    if not key:
        return ("C05", "SKIP", reason)

    ctx.client.set_global_setting("api_mode_enabled", True)
    pid, _root = make_project(ctx)

    sid = con.create_chat(ctx, pid, engine="api", model=OPENAI_MODEL)
    try:
        con.send_message(ctx, sid, "Reply with exactly the single word: pong")
        reply = con.wait_assistant_reply(ctx, sid, contains="pong")
        detail = con.wait_turn_idle(ctx, sid)
        if detail.get("engine") != "api":
            return ("C05", "FAIL", f"detail engine = {detail.get('engine')!r}")
    finally:
        con.close_chat(ctx, sid)

    row = next((r for r in con.list_chats(ctx, pid)
                if r["session_id"] == sid), {})
    if row.get("live") or row.get("status") != "interactive_completed":
        return ("C05", "FAIL", f"closed chat row = {row}")

    return ("C05", "PASS", f"openai api chat roundtrip ok (reply {len(reply)} chars)")
