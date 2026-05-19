"""Manual-testing scenarios for the python (script-mode) provider.

These exercise execution_mode='script' agents — plain python3 driving the
workflow via the embedded nrflo_sdk over the agent socket. No provider
CLI, no LLM."""

from __future__ import annotations

from . import (
    P01_findings_basic,
    P02_findings_append,
    P03_findings_get_filters,
    P04_findings_delete,
    P05_layer_findings_get,
    P06_project_findings,
    P07_project_findings_xwf,
    P08_agent_fail,
    P09_agent_continue,
    P10_agent_callback,
    P11_context_user_instructions,
    P12_log_categories,
    P13_skip,
    P14_project_env_var,
    P15_exception_failure,
    P16_stderr_captured,
    P17_multi_skip_tag,
    P18_chain_require_ticket_handoff,
    P19_notification_accessor,
    P20_seed_findings,
)


ALL_SCENARIOS = [
    P01_findings_basic.run,
    P02_findings_append.run,
    P03_findings_get_filters.run,
    P04_findings_delete.run,
    P05_layer_findings_get.run,
    P06_project_findings.run,
    P07_project_findings_xwf.run,
    P08_agent_fail.run,
    P09_agent_continue.run,
    P10_agent_callback.run,
    P11_context_user_instructions.run,
    P12_log_categories.run,
    P13_skip.run,
    P14_project_env_var.run,
    P15_exception_failure.run,
    P16_stderr_captured.run,
    P17_multi_skip_tag.run,
    P18_chain_require_ticket_handoff.run,
    P19_notification_accessor.run,
    P20_seed_findings.run,
]
