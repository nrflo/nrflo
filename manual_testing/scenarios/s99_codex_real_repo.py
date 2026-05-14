"""S99 — DIAGNOSTIC: codex/cli_interactive with an existing real repo as workdir.

Hypothesis: the regular harness's `make_project` creates a fresh `git init`
with only `.gitkeep` committed; codex 0.130.0 in PTY hangs silently on that
minimal repo state. Same harness with a real, populated repo as workdir
should reach `nrflo agent finished` normally.

This scenario bypasses `make_project` and points the project at
`~/projects/2026/3dgmcv` directly (a real repo, trusted in the user's
~/.codex/config.toml). Run it manually:

  python3 manual_testing/test_codex.py \
      --only=s99_codex_real_repo --parallel=1

NOT part of ALL_SCENARIOS — diagnostic only. Delete after the codex
PTY-on-minimal-repo investigation closes.
"""

from __future__ import annotations

from pathlib import Path

from lib import db as db_mod
from lib.runtime import (
    Ctx, PASS_STATUSES, Result,
    first_session, next_id, resolve_model, wait_for_workflow,
)


MODELS_BY_PROVIDER: dict[str, str] = {}

REAL_REPO = Path("/Users/anderfred/projects/2026/3dgmcv")

PROMPT = """\
You are an integration-test agent. Do EXACTLY what is listed below and
nothing else. Use the Bash tool to run the listed commands in order,
then stop immediately.

1. Run: `nrflo findings add greeting hello`
2. Run: `nrflo agent finished`
"""


def run(ctx: Ctx) -> Result:
    if not REAL_REPO.exists():
        return ("S99 codex real repo", "SKIP",
                f"{REAL_REPO} does not exist on this host")

    pid = next_id(ctx, "p-real")
    ctx.client.create_project(pid, root_path=str(REAL_REPO))
    wid = next_id(ctx, "wf")
    ctx.client.create_workflow(pid, wid, scope_type="project")
    ctx.client.create_agent_def(
        pid, wid, "main",
        model=resolve_model(ctx, MODELS_BY_PROVIDER),
        layer=0, timeout=5, prompt=PROMPT,
    )
    wfi = ctx.client.run_project_workflow(
        pid, wid, instructions="real-repo diagnostic",
    )["instance_id"]
    wait_for_workflow(ctx, pid, instance_id=wfi)

    sess = first_session(db_mod.agent_sessions_for_instance(ctx.server.home, wfi))
    if sess["status"] not in PASS_STATUSES or sess["result"] != "pass":
        return ("S99 codex real repo", "FAIL",
                f"session status/result = {sess['status']}/{sess['result']}")
    greeting = (sess.get("findings") or {}).get("greeting")
    if greeting != "hello":
        return ("S99 codex real repo", "FAIL",
                f"findings.greeting = {greeting!r}")
    return ("S99 codex real repo", "PASS",
            f"completed in real repo workdir; session={sess['id']}")
