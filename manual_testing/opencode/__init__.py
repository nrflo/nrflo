"""Manual-testing scenarios specific to the opencode CLI provider.

Engine-level scenarios live in `manual_testing/engine/`. This folder
holds only the scenarios whose implementation diverges per provider —
including `s27`, opencode's agent-saver branch of the low-context
relaunch (engine covers the native-resume branch via `s26`).
"""

from __future__ import annotations

from . import (
    s05_context_left,
    s27_context_save_agent_saver,
    s35_custom_cli_model,
)


ALL_SCENARIOS = [
    s05_context_left.run,
    s27_context_save_agent_saver.run,
    s35_custom_cli_model.run,
]
