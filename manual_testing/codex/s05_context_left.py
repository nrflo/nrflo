"""S05 — context_left populated (codex via app-server).

codex/cli_interactive is driven by the `codex app-server` JSON-RPC backend
(be/internal/spawner/codex_appserver_backend.go), not the PTY/TUI. codex 0.133
exposes no usable structured channel under PTY (hooks never fire per
openai/codex#21639; no rollout JSONL is written), so the app-server protocol is
the source of truth. Its `thread/tokenUsage/updated` events carry
`inputTokens` + `modelContextWindow`, which the backend maps to
`agent_sessions.context_left` via ComputeContextLeftPct — restoring the
context tracking (and low-context relaunch) that the PTY path could not provide.

This scenario also guards the regression that broke codex on 0.133 (the agent
hung forever on the directory-trust dialog → endless start-stall loop): it
asserts the workflow actually completes AND that context_left is populated,
proving the structured backend is wired.

Expected result:
  - PASS  workflow project_completed, session result=pass, context_left ∈ [0,100]
  - FAIL  workflow did not complete, or context_left NULL (token-usage events
          not mapped — app-server backend not wired)
"""

from __future__ import annotations

from lib import db as db_mod
from lib.runtime import (
    Ctx, PASS_STATUSES, Result,
    first_session, make_project, next_id, resolve_model, wait_for_workflow,
)


# Per-provider model overrides; empty = use the runner default.
MODELS_BY_PROVIDER: dict[str, str] = {}


PROMPT = """\
You are an integration-test agent. Follow the steps below exactly,
in order, then stop.

1. List 8 single-line architectural tradeoffs (one per line) when
   choosing between strong and eventual consistency for a distributed
   key-value store. Each line should be ~10-15 words. Output as plain
   text — no files. Keep total under ~150 words so reasoning-heavy
   providers finish quickly.
2. Use the Bash tool to run: `nrflo agent finished`
"""


def run(ctx: Ctx) -> Result:
    pid, _root = make_project(ctx)
    wid = next_id(ctx, "wf")
    ctx.client.create_workflow(pid, wid, scope_type="project")
    ctx.client.create_agent_def(
        pid, wid, "main",
        model=resolve_model(ctx, MODELS_BY_PROVIDER), layer=0, timeout=5, prompt=PROMPT,
    )
    wfi = ctx.client.run_project_workflow(
        pid, wid, instructions="context left",
    )["instance_id"]
    wait_for_workflow(ctx, pid, instance_id=wfi)

    sess = first_session(db_mod.agent_sessions_for_instance(ctx.server.home, wfi))
    if sess["status"] not in PASS_STATUSES or sess["result"] != "pass":
        return ("S05 context_left", "FAIL",
                f"session status/result = {sess['status']}/{sess['result']} "
                "(trust-dialog hang / start-stall loop?)")
    cl = sess.get("context_left")
    if cl is None:
        return ("S05 context_left", "FAIL",
                "context_left NULL — app-server token-usage events not mapped?")
    if not (0 <= cl <= 100):
        return ("S05 context_left", "FAIL", f"context_left out of range: {cl}")
    return ("S05 context_left", "PASS", f"context_left={cl}")
