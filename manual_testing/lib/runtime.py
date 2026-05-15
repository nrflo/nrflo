"""Shared runtime types + helpers for scenarios.

`Ctx` carries the running server, REST client, provider, model, binary,
and a per-scenario label used in logs. Scenarios call `make_project(ctx)`
to get a fresh isolated project. The runner sets
`NrfloClient.default_execution_mode` to the current mode once so every
`create_agent_def` call carries `execution_mode` to the API without
per-scenario boilerplate."""

from __future__ import annotations

import itertools
import subprocess
import threading
import time
from dataclasses import dataclass
from pathlib import Path

from . import api as api_mod
from . import server as server_mod


Result = tuple[str, str, str]  # (name, "PASS"|"FAIL"|"SKIP", details)

TERMINAL = {"completed", "failed", "project_completed"}
RUN_TIMEOUT_S = 180.0
POLL_INTERVAL_S = 0.5  # halved from 1.0 — REST cost is negligible vs wall-time savings

MODE = "cli_interactive"


@dataclass
class Ctx:
    server: server_mod.RunningServer
    client: api_mod.NrfloClient
    provider: str           # claude | codex | gemini | opencode | python
    model: str              # cli_models row id (e.g. "haiku")
    binary: str             # PATH binary name
    mode: str = "cli_interactive"   # "cli_interactive" for CLI; "script" for python
    scenario: str = ""      # set per-scenario for log prefixing


_id_lock = threading.Lock()
_id_counter = itertools.count(1)


def next_id(ctx: Ctx, kind: str) -> str:
    with _id_lock:
        n = next(_id_counter)
    return f"{kind}-{ctx.provider}-{n}"


def resolve_model(ctx: Ctx, overrides: dict[str, str] | None) -> str:
    """Return a per-provider model override (when set) or the default
    `ctx.model`. Scenarios that need a stronger model on a particular
    provider declare a module-level dict like:

        MODELS_BY_PROVIDER = {"claude": "sonnet", "codex": "codex_gpt_high"}

    and pass it to `resolve_model(ctx, MODELS_BY_PROVIDER)` when building
    their agent_def. Empty/None dict means "use whatever the runner picked"
    (the entry script's default, e.g. claude=haiku)."""
    if not overrides:
        return ctx.model
    return overrides.get(ctx.provider, ctx.model)


def make_project(ctx: Ctx) -> tuple[str, Path]:
    """Create a fresh project. Mode-specific behavior is now applied at
    agent-definition time via `NrfloClient.default_execution_mode` (set
    once by the runner) — no project-level toggle is needed."""
    pid = next_id(ctx, "p")
    root = ctx.server.home / "projects" / pid
    root.mkdir(parents=True, exist_ok=True)
    subprocess.run(["git", "init", "-q", str(root)], check=False)
    (root / ".gitkeep").write_text("")
    subprocess.run(["git", "-C", str(root), "add", "."], check=False,
                   capture_output=True)
    subprocess.run(
        ["git", "-C", str(root), "-c", "user.email=t@t", "-c", "user.name=t",
         "commit", "-q", "-m", "init"],
        check=False, capture_output=True,
    )
    ctx.client.create_project(pid, root_path=str(root))
    return pid, root


def pick_instance(state: dict, instance_id: str) -> dict | None:
    all_wfs = state.get("all_workflows") or {}
    if instance_id in all_wfs:
        return all_wfs[instance_id]
    if state.get("instance_id") == instance_id:
        return state
    return None


def wait_for_workflow(
    ctx: Ctx,
    pid: str,
    *,
    instance_id: str,
    ticket_id: str | None = None,
) -> dict:
    deadline = time.monotonic() + RUN_TIMEOUT_S
    start = time.monotonic()
    last: dict = {}
    last_status: str | None = None
    tag = f"        [{ctx.scenario or '?'}]"
    print(f"{tag} polling workflow {instance_id[:8]}… "
          f"(project={pid}, ticket={ticket_id or '-'})", flush=True)
    while time.monotonic() < deadline:
        if ticket_id:
            last = ctx.client.get_ticket_workflow_state(
                pid, ticket_id, instance_id=instance_id)
        else:
            last = ctx.client.get_project_workflow_state(
                pid, instance_id=instance_id)
        wf = pick_instance(last, instance_id)
        status = (wf or {}).get("status") if wf else None
        if status != last_status:
            elapsed = time.monotonic() - start
            print(f"{tag} status={status} (after {elapsed:.1f}s)", flush=True)
            last_status = status
        if wf and status in TERMINAL:
            return last
        time.sleep(POLL_INTERVAL_S)
    try:
        ctx.client.stop_project_workflow(pid, instance_id=instance_id)
    except Exception:
        pass
    raise TimeoutError(f"workflow {instance_id} did not finish in {RUN_TIMEOUT_S}s")


def run_single_agent(
    ctx: Ctx,
    *,
    prompt: str,
    model: str | None = None,
    timeout: int = 5,
    instructions: str = "run the test",
) -> tuple[str, str, dict]:
    """Project + project-scope workflow + one L0 agent → fire run → wait
    until terminal. Returns (project_id, instance_id, state)."""
    pid, _root = make_project(ctx)
    wid = next_id(ctx, "wf")
    ctx.client.create_workflow(pid, wid, scope_type="project")
    ctx.client.create_agent_def(
        pid, wid, "main",
        model=model or ctx.model,
        prompt=prompt,
        layer=0,
        timeout=timeout,
    )
    run = ctx.client.run_project_workflow(pid, wid, instructions=instructions)
    instance_id = run["instance_id"]
    state = wait_for_workflow(ctx, pid, instance_id=instance_id)
    return pid, instance_id, state


def first_session(sessions: list[dict]) -> dict:
    if not sessions:
        raise AssertionError("no agent_sessions row was created")
    return sessions[0]


PASS_STATUSES = {"completed", "project_completed"}
