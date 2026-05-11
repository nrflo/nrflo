"""Provider/mode-agnostic manual-testing scenarios.

Each module in this package exposes a single `run(ctx) -> Result` function
plus inline PROMPT/agent configuration so the test setup is self-contained
in one file. The runner (`lib.runner.run_all`) calls them in order using
`ALL_SCENARIOS` below. Comment out or reorder entries to filter."""

from __future__ import annotations

from . import (
    s01_findings_save,
    s02_agent_fail,
    s03_project_findings,
    s04_message_categories,
    s05_context_left,
    s06_skip_tag,
    s07_layer_handoff,
    s08_workflow_final_result,
    s09_ticket_scope,
    s10_parallel_agents,
    s11_user_instructions,
    s12_project_findings_xwf,
    s13_project_env_var,
    s14_pass_policy_all,
    s15_prior_layer_findings,
    s16_stall_detection,
    s17_callback,
    s18_retry_failed,
    s19_endless_loop,
    s20_chain_run,
    s21_next_workflow_on_success,
    s22_max_fail_restarts,
    s23_chain_next_ticket,
    s24_agent_session_logs,
)


ALL_SCENARIOS = [
    s01_findings_save.run,
    s02_agent_fail.run,
    s03_project_findings.run,
    s04_message_categories.run,
    s05_context_left.run,
    s06_skip_tag.run,
    s07_layer_handoff.run,
    s08_workflow_final_result.run,
    s09_ticket_scope.run,
    s10_parallel_agents.run,
    s11_user_instructions.run,
    s12_project_findings_xwf.run,
    s13_project_env_var.run,
    s14_pass_policy_all.run,
    s15_prior_layer_findings.run,
    s16_stall_detection.run,
    s17_callback.run,
    s18_retry_failed.run,
    s19_endless_loop.run,
    s20_chain_run.run,
    s21_next_workflow_on_success.run,
    s22_max_fail_restarts.run,
    s23_chain_next_ticket.run,
    s24_agent_session_logs.run,
]
