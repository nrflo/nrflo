"""Manual-testing scenarios specific to api-mode (execution_mode='api').

These scenarios drive `be/internal/spawner/apirun.Runner` against a real
Anthropic endpoint authenticated by the OAuth bearer token resolved from
the macOS Keychain by `lib/credentials.py`. The whole folder SKIPs
cleanly when no token is reachable.

The cli_interactive providers (claude/, codex/, gemini/, opencode/) have
no equivalent of the apirun-specific code paths exercised here:
api_mode_enabled gating, in-process builtin tool dispatch, terminal-signal
mapping, forced agent-save on low-context, and the auth-error result_reason.
"""

from __future__ import annotations

from . import (
    A01_hello_world,
    A02_agent_fail,
    A03_agent_continue,
    A04_agent_callback,
    A05_api_mode_disabled,
    A06_findings_tool,
    A07_project_findings_xwf,
    A08_low_context_agent_save,
    A09_stall_restart,
    A10_auth_error,
    A11_python_tool_dispatch,
    A12_api_mode_toggle,
)


ALL_SCENARIOS = [
    A01_hello_world.run,
    A02_agent_fail.run,
    A03_agent_continue.run,
    A04_agent_callback.run,
    A05_api_mode_disabled.run,
    A06_findings_tool.run,
    A07_project_findings_xwf.run,
    A08_low_context_agent_save.run,
    A09_stall_restart.run,
    A10_auth_error.run,
    A11_python_tool_dispatch.run,
    A12_api_mode_toggle.run,
]
