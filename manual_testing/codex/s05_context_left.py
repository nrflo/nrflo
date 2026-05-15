"""S05 — context_left populated.

Tests:
  - The CLI integration reports remaining context % to the server,
    populating agent_sessions.context_left.
  - claude: PreToolUse/PostToolUse hooks via `agent.context_update`.
  - codex: rollout JSONL tailer (`cli_adapter_codex_jsonl_tail.go`).
  - opencode: SQLite tailer (`cli_adapter_opencode_sqlite_tail.go`).

Why the prompt asks for substantial output:
  - A trivial `nrflo agent finished`-only prompt consumes essentially
    no input/output tokens, and some adapters (notably opencode) only
    flush their per-turn token bookkeeping at end-of-turn via async
    DB writes. Under parallel load that flush can be dropped entirely
    for zero-work turns, leaving context_left=NULL even though the
    pipeline is healthy.
  - Forcing the model to produce a real response (~400-800 tokens of
    output + reasoning) gives every adapter a real telemetry event
    to record and exercises the actual code path users hit.

Expected result:
  - PASS  agent_sessions.context_left ∈ [0, 100]
  - FAIL  context_left NULL — codex rollout JSONL tailer not wired.
"""

from __future__ import annotations

from lib import db as db_mod
from lib.runtime import (
    Ctx, Result, first_session, make_project, next_id, resolve_model, wait_for_workflow,
)


# Per-provider model overrides; empty = use the runner default (e.g. haiku).
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
    cl = sess.get("context_left")
    if cl is None:
        return ("S05 context_left", "FAIL",
                "context_left NULL — codex rollout JSONL tailer not wired?")
    if not (0 <= cl <= 100):
        return ("S05 context_left", "FAIL", f"context_left out of range: {cl}")
    return ("S05 context_left", "PASS", f"context_left={cl}")
