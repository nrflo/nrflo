"""S05 — context_left populated.

Tests:
  - The CLI integration reports remaining context % to the server,
    populating agent_sessions.context_left.
  - claude: PreToolUse/PostToolUse hooks via `agent.context_update`.
  - codex: rollout JSONL tailer (`cli_adapter_codex_jsonl_tail.go`).
  - opencode: SQLite tailer (`cli_adapter_opencode_sqlite_tail.go`).

Expected result:
  - PASS  agent_sessions.context_left ∈ [0, 100]
  - SKIP  script backend only (scriptBackend.TracksContext() = false)
  - FAIL  any CLI provider × mode that leaves it NULL — tailers wire it
          for every cli/cli_interactive combo.
"""

from __future__ import annotations

from lib import db as db_mod
from lib.runtime import (
    Ctx, Result, first_session, make_project, next_id, resolve_model, wait_for_workflow,
)


# Per-provider model overrides; empty = use the runner default (e.g. haiku).
MODELS_BY_PROVIDER: dict[str, str] = {}


PROMPT = """\
You are an integration-test agent. Do EXACTLY what is listed below and
nothing else. Use the Bash tool to run the listed command, then stop.

1. Run: `nrflo agent finished`
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
        if ctx.provider == "script":
            return ("S05 context_left", "SKIP",
                    "script backend does not track context")
        return ("S05 context_left", "FAIL",
                f"{ctx.provider}/{ctx.mode} left context_left NULL — "
                "tailer/hook not wired?")
    if not (0 <= cl <= 100):
        return ("S05 context_left", "FAIL", f"context_left out of range: {cl}")
    return ("S05 context_left", "PASS", f"context_left={cl}")
