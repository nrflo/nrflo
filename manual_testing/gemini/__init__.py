"""Manual-testing scenarios specific to the gemini CLI provider.

Engine-level scenarios live in `manual_testing/engine/`. This folder
holds only the scenarios whose implementation diverges per provider.
"""

from __future__ import annotations

from . import (
    s05_context_left,
    s35_custom_cli_model,
)


ALL_SCENARIOS = [
    s05_context_left.run,
    s35_custom_cli_model.run,
]
