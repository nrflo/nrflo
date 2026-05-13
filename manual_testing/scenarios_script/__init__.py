"""Script-mode scenarios for execution_mode='script' agents.

Each module exposes a single `run(ctx) -> Result`. Scenarios live in
their own namespace (parallel to `scenarios/`) because they target the
provider/model-agnostic Python script backend
(`be/internal/spawner/backend_script.go`) and exercise the embedded
SDK (`be/internal/sdk/python/nrflo_sdk.py`) — no LLM, no provider CLI.

Naming: `psNN_<short_name>.py` (ps = python-script). The list below
controls execution order; comment out to skip."""

from __future__ import annotations

from . import (
    ps01_findings_basic,
    ps02_findings_append,
    ps03_findings_get_filters,
    ps04_findings_delete,
    ps05_layer_findings_get,
    ps06_project_findings,
    ps07_project_findings_xwf,
    ps08_agent_fail,
    ps09_agent_continue,
    ps10_agent_callback,
    ps11_context_user_instructions,
    ps12_log_categories,
    ps13_skip,
    ps14_project_env_var,
    ps15_exception_failure,
    ps16_stderr_captured,
)


ALL_SCRIPT_SCENARIOS = [
    ps01_findings_basic.run,
    ps02_findings_append.run,
    ps03_findings_get_filters.run,
    ps04_findings_delete.run,
    ps05_layer_findings_get.run,
    ps06_project_findings.run,
    ps07_project_findings_xwf.run,
    ps08_agent_fail.run,
    ps09_agent_continue.run,
    ps10_agent_callback.run,
    ps11_context_user_instructions.run,
    ps12_log_categories.run,
    ps13_skip.run,
    ps14_project_env_var.run,
    ps15_exception_failure.run,
    ps16_stderr_captured.run,
]
