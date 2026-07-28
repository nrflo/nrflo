"""C06 — api-engine native bash through the approval flow.

`api_native_tools_enabled` injects the workdir-jailed FSTools set (8 tools:
read_file/edit_file/write_file/glob/grep/bash/bash_output/kill_shell) into
an api console chat, with the mutating ones (edit_file/bash) gated behind
the human approval flow. Chats default yolo ON, so this scenario turns it
OFF first to force the approval round-trip. Ask the model to run one bash
command that both writes a probe file and echoes a nonce; the wait loop
answers each approval with allow_for_session, so afterwards the detail
snapshot's `session_approvals` must list `bash` — that asserts the
approval round-trip (and the session-allowlist surface) rather than
trusting the reply text alone. The on-disk probe file is the hard evidence
the command actually executed in the project root. SKIPs without an
Anthropic OAuth token, same as C03.
"""

from __future__ import annotations

from lib import console as con
from lib.credentials import probe_oauth_token
from lib.runtime import Ctx, Result, make_project, next_id

API_MODEL = "haiku-4-5"


def run(ctx: Ctx) -> Result:
    tok, reason = probe_oauth_token()
    if not tok:
        return ("C06", "SKIP", reason)

    ctx.client.set_global_setting("api_mode_enabled", True)
    ctx.client.set_global_setting("api_native_tools_enabled", True)
    pid, root = make_project(ctx)
    nonce = next_id(ctx, "c06nonce")

    sid = con.create_chat(ctx, pid, engine="api", model=API_MODEL)
    try:
        con.set_yolo(ctx, sid, False)
        con.send_message(
            ctx, sid,
            "Use the bash tool to run exactly this command, then reply with "
            f"the command's stdout: echo {nonce} | tee c06_probe.txt")
        reply = con.wait_assistant_reply(ctx, sid, contains=nonce,
                                         approve="allow_for_session")
        detail = con.wait_turn_idle(ctx, sid)

        allowed = detail.get("session_approvals") or []
        if "bash" not in allowed:
            return ("C06", "FAIL",
                    f"bash missing from session_approvals after "
                    f"allow_for_session: {allowed!r}")

        probe = root / "c06_probe.txt"
        if not probe.exists():
            return ("C06", "FAIL", f"probe file not written: {probe}")
        content = probe.read_text().strip()
        if nonce not in content:
            return ("C06", "FAIL",
                    f"probe content {content!r} missing nonce {nonce!r}")
    finally:
        con.close_chat(ctx, sid)

    return ("C06", "PASS",
            f"bash approval roundtrip ok: probe file written, "
            f"session_approvals={allowed} (reply {len(reply)} chars)")
