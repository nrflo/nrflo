"""S47 — CLI rate-limit detection broadcasts agent.rate_limited + flips DB.

Tests the cli_interactive rate-limit path (be/internal/spawner/rate_limit_restart.go,
be/internal/spawner/cli_adapter_claude.go:95): when claude exits non-zero
with a recognised limit pattern in recent output, the spawner broadcasts
`agent.rate_limited`, registers the stop as continue/rate_limit, and
persists `rate_limit_retry_count` + `rate_limit_until_ts` on the session row.

Strategy: spin up a private nrflo_server with a `claude` stub at the head
of PATH. The stub prints "You've hit your limit" (matches the default
`claude_limit_patterns` in `ClaudeAdapter.ClassifyExit`) and exits non-zero.
Go's exec.LookPath resolves `claude` against the server-process PATH at
Command construction time, so this is the only reliable way to substitute
the binary without per-scenario env hooks. The server is local to the
scenario; other engine scenarios keep using the real claude binary.

The spawner's default backoff is 60s. The `agent.rate_limited` WS event
is broadcast BEFORE the wait, so the scenario observes it within a few
seconds, then stops the workflow (cancels ctx → `waitForRateLimitRetry`
returns false → no relaunch). DB columns are inspected after stop.

Expected PASS:
  - WS receives `agent.rate_limited` with the matched pattern + retry_count=1
  - agent_sessions.rate_limit_retry_count == 1
  - agent_sessions.last_retry_class contains "limit" (matched pattern)
"""

from __future__ import annotations

import importlib.util
import os
import stat
import tempfile
from pathlib import Path

from lib import db as db_mod
from lib.api import NrfloClient
from lib.runtime import Ctx, Result, next_id
from lib.server import start_server
from lib.ws_client import WSClient


PROMPT = """\
You are an integration-test agent. Run the listed command and stop.

1. Run: `nrflo agent finished`
"""


def _make_stub_claude() -> Path:
    """Return a dir containing a `claude` stub that triggers ClassifyExit
    → RetryClassRateLimit on the very first spawn."""
    stub_dir = Path(tempfile.mkdtemp(prefix="s47-claude-stub-"))
    stub = stub_dir / "claude"
    # Print the canonical limit pattern + exit non-zero. Anything written
    # to stdout reaches the PTY ferry's recent-blocks ring buffer, which
    # `ClassifyExit` reads in `handleCompletion`.
    stub.write_text(
        "#!/bin/sh\n"
        "printf \"You've hit your limit\\n\"\n"
        "exit 1\n"
    )
    stub.chmod(stub.stat().st_mode | stat.S_IXUSR | stat.S_IXGRP | stat.S_IXOTH)
    return stub_dir


def run(ctx: Ctx) -> Result:
    if importlib.util.find_spec("websockets") is None:
        return ("S47 CLI rate-limit", "SKIP",
                "websockets package not installed (pip install websockets)")

    stub_dir = _make_stub_claude()
    # Put the stub first so Go's exec.LookPath in the server picks it up
    # instead of the real `claude` binary on the developer's PATH.
    extra_path = f"{stub_dir}{os.pathsep}{os.environ.get('PATH', '')}"
    sub = start_server(
        cli_label="engine-s47-rl",
        extra_env={"PATH": extra_path},
    )
    try:
        client = NrfloClient(sub.base_url)
        client.login()

        pid = next_id(ctx, "p-s47")
        root = sub.home / "projects" / pid
        root.mkdir(parents=True, exist_ok=True)
        (root / ".gitkeep").write_text("")
        # No git init needed for the spawner; the project root just has to exist.
        client.create_project(pid, root_path=str(root))
        wid = next_id(ctx, "wf-s47")
        client.create_workflow(pid, wid, scope_type="project")
        client.create_agent_def(
            pid, wid, "main",
            model="haiku", layer=0, timeout=30, prompt=PROMPT,
        )

        with WSClient(sub.base_url, client._jar) as ws:
            ws.subscribe(pid, since_seq=None)
            ack = ws.wait_for(
                lambda e: (e.get("type") == "ack"
                           and e.get("action") == "subscribed"
                           and e.get("project_id") == pid),
                timeout_s=10.0,
            )
            if ack is None:
                return ("S47 CLI rate-limit", "FAIL",
                        "WS subscribe ack not received within 10s")
            wfi = client.run_project_workflow(
                pid, wid, instructions="s47 rate-limit",
            )["instance_id"]

            ev = ws.wait_for(
                lambda e: (e.get("type") == "agent.rate_limited"
                           and e.get("project_id") == pid),
                timeout_s=30.0,
            )

        # Always stop the workflow so the spawner's backoff sleep doesn't
        # hold up the test; ctx cancellation aborts waitForRateLimitRetry.
        try:
            client.stop_project_workflow(pid, instance_id=wfi)
        except Exception:
            pass

        if ev is None:
            return ("S47 CLI rate-limit", "FAIL",
                    "no agent.rate_limited WS event within 30s")
        data = ev.get("data") or {}
        if data.get("retry_count") != 1:
            return ("S47 CLI rate-limit", "FAIL",
                    f"retry_count = {data.get('retry_count')!r}, want 1")
        matched = data.get("matched_pattern") or ""
        if "limit" not in matched.lower():
            return ("S47 CLI rate-limit", "FAIL",
                    f"matched_pattern = {matched!r}, expected to contain 'limit'")

        # DB columns must reflect the retry bookkeeping. Wait briefly for
        # the spawner goroutine to flush UpdateRateLimitUntil before stop.
        import time as _t
        deadline = _t.monotonic() + 5.0
        sess = None
        while _t.monotonic() < deadline:
            sessions = db_mod.agent_sessions_for_instance(sub.home, wfi)
            if sessions and (sessions[0].get("rate_limit_retry_count") or 0) >= 1:
                sess = sessions[0]
                break
            _t.sleep(0.1)
        if sess is None:
            return ("S47 CLI rate-limit", "FAIL",
                    "rate_limit_retry_count did not reach 1 within 5s of WS event")
        if not (sess.get("last_retry_class") or ""):
            return ("S47 CLI rate-limit", "FAIL",
                    f"last_retry_class empty; sess = {dict(sess)!r}")

        return ("S47 CLI rate-limit", "PASS",
                f"ws=agent.rate_limited retry_count={data.get('retry_count')} "
                f"matched={matched!r} db.retry={sess['rate_limit_retry_count']}")
    finally:
        sub.stop(keep_dir=True)
        import shutil
        shutil.rmtree(stub_dir, ignore_errors=True)
