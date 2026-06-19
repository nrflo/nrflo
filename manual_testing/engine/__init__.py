"""Engine-level manual-testing scenarios.

Provider-agnostic scenarios that exercise the orchestrator, REST, WS,
spawner, DB, chains, retries, stall detection, plan mode, etc. Run under
the `claude` binary by default — see `engine/test.py`.

Genuinely provider-specific scenarios (`s05`, `s35`) live in the
per-provider folders.
"""

from __future__ import annotations

from . import (
    s01_l0_happy_path,
    s02_agent_fail,
    s07_template_expansion,
    s09_ticket_scope,
    s10_parallel_agents,
    s12_project_findings_xwf,
    s14_pass_policy_all,
    s16_stall_detection,
    s17_callback,
    s18_retry_failed,
    s19_endless_loop,
    s20_chain_run,
    s21_next_workflow_on_success,
    s22_max_fail_restarts,
    s23_chain_next_ticket,
    s25_findings_carryover,
    s26_context_save_resume,
    s29_manual_restart,
    s30_ticket_concurrency_guard,
    s31_take_control_exit_interactive,
    s32_plan_mode,
    s34_multi_instance_same_ticket,
    s37_ws_event_subscriber,
    s38_notification_webhook,
    s39_validation_commands_pass,
    s40_validation_commands_fail,
    s41_workflow_export_import,
    s42_service_token,
    s43_artifacts_e2e,
    s45_notification_script,
    s46_observer,
    s47_rate_limit_cli,
    s48_ws_query_token,
    s49_purge_on_completion,
)


ALL_SCENARIOS = [
    s01_l0_happy_path.run,
    s02_agent_fail.run,
    s07_template_expansion.run,
    s09_ticket_scope.run,
    s10_parallel_agents.run,
    s12_project_findings_xwf.run,
    s14_pass_policy_all.run,
    s16_stall_detection.run,
    s17_callback.run,
    s18_retry_failed.run,
    s19_endless_loop.run,
    s20_chain_run.run,
    s21_next_workflow_on_success.run,
    s22_max_fail_restarts.run,
    s23_chain_next_ticket.run,
    s25_findings_carryover.run,
    s26_context_save_resume.run,
    s29_manual_restart.run,
    s30_ticket_concurrency_guard.run,
    s31_take_control_exit_interactive.run,
    s32_plan_mode.run,
    s34_multi_instance_same_ticket.run,
    s37_ws_event_subscriber.run,
    s38_notification_webhook.run,
    s39_validation_commands_pass.run,
    s40_validation_commands_fail.run,
    s41_workflow_export_import.run,
    s42_service_token.run,
    s43_artifacts_e2e.run,
    s45_notification_script.run,
    s46_observer.run,
    s47_rate_limit_cli.run,
    s48_ws_query_token.run,
    s49_purge_on_completion.run,
]
