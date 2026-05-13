"""Helpers for execution_mode='script' scenarios.

Script-mode agents run plain Python via the spawner's scriptBackend
(`be/internal/spawner/backend_script.go`). Each script bootstraps the
embedded SDK from `$NRFLO_SDK_DIR`, then drives the workflow via
`nrflo_sdk.client()` — findings, agent.finished/fail/continue/callback,
context, skip, log. No LLM, no provider CLI."""

from __future__ import annotations

import textwrap

from .runtime import Ctx, next_id


# Boilerplate prepended to every script body so test scenarios only have
# to describe what the agent does, not how to import the SDK.
SDK_BOOTSTRAP = textwrap.dedent("""\
    import os
    import sys
    sys.path.insert(0, os.environ["NRFLO_SDK_DIR"])
    import nrflo_sdk
    c = nrflo_sdk.client()
""")


def make_script(
    ctx: Ctx,
    pid: str,
    *,
    name: str | None = None,
    code: str,
) -> str:
    """Create a python_script row and return its server-assigned id.

    `code` is wrapped with the SDK_BOOTSTRAP prelude so callers can write
    plain SDK calls and assume `c` is bound to a nrflo_sdk.Client."""
    script = ctx.client.create_python_script(
        pid,
        name=name or next_id(ctx, "ps"),
        code=SDK_BOOTSTRAP + textwrap.dedent(code),
    )
    return script["id"]


def make_script_agent(
    ctx: Ctx,
    pid: str,
    wid: str,
    agent_id: str,
    *,
    code: str,
    layer: int = 0,
    timeout: int = 2,
    model: str = "haiku",
    stall_start_timeout_sec: int | None = None,
    stall_running_timeout_sec: int | None = None,
    max_fail_restarts: int | None = None,
) -> str:
    """One-shot: create python_script + script-mode agent_def.

    Returns the python_script id. `model` is required by the agent_def
    schema but is unused by the script backend; defaults to 'haiku' so
    cli_model lookup in the spawner (if any) finds a valid row."""
    script_id = make_script(ctx, pid, code=code)
    ctx.client.create_agent_def(
        pid, wid, agent_id,
        model=model,
        layer=layer,
        timeout=timeout,
        execution_mode="script",
        python_script_id=script_id,
        stall_start_timeout_sec=stall_start_timeout_sec,
        stall_running_timeout_sec=stall_running_timeout_sec,
        max_fail_restarts=max_fail_restarts,
    )
    return script_id
