"""Manual-testing scenarios for api-mode against a real OpenAI Responses endpoint.

These scenarios run execution_mode='api' authenticated by the API key resolved
from probe_openai_key() (OPENAI_API_KEY → CODEX_API_KEY, env only, no Keychain).
The whole folder SKIPs cleanly when neither key is set.

Provider-agnostic scenarios A01/A02/A03/A06 are reused unchanged from api/;
O01 is the openai-only reasoning-effort scenario.
"""

from __future__ import annotations

from api import (
    A01_hello_world,
    A02_agent_fail,
    A03_agent_continue,
    A06_findings_tool,
)
from openai_api import O01_reasoning_effort


ALL_SCENARIOS = [
    A01_hello_world.run,
    A02_agent_fail.run,
    A03_agent_continue.run,
    A06_findings_tool.run,
    O01_reasoning_effort.run,
]
