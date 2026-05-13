"""S28 — Codex resume → agent-saver fallback (deferred).

The fallback path in `be/internal/spawner/context_save_resume.go:29-150`
fires when a `codex exec resume <thread_id>` call fails (missing
thread_id, non-zero exit, timeout, missing `to_resume` findings after a
clean exit). Without a production-code test seam (e.g. an env-gated
`NRFLO_FORCE_RESUME_FAIL=1`) the harness cannot reliably trigger the
failure: corrupting the thread_id mid-flight depends on internal state
and slows every run for no integration coverage gain.

Decision (recorded in `backlog.md`): defer. The fallback path is
unit-tested in `be/internal/spawner/context_save_resume_test.go`. Add
the env seam + flip this stub to a real scenario only if the unit
coverage degrades.
"""

from __future__ import annotations

from lib.runtime import Ctx, Result


def run(ctx: Ctx) -> Result:
    return ("S28 codex resume fallback", "SKIP",
            "deferred — needs NRFLO_FORCE_RESUME_FAIL seam; see backlog.md")
