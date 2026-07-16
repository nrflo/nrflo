"""Console-chat scenarios (C-prefix): the server-owned console surface
(POST /api/v1/console/chats + catalog/list/detail/messages/close), one
scenario per engine plus a tool-dispatch check. Engine-scoped, not
provider-scoped — each file gates on its own binary/credentials."""

from . import (
    C01_claude_chat_roundtrip,
    C02_codex_chat_roundtrip,
    C03_api_chat_roundtrip,
    C04_claude_chat_tools,
    C05_openai_api_chat_roundtrip,
)

ALL_SCENARIOS = [
    C01_claude_chat_roundtrip.run,
    C02_codex_chat_roundtrip.run,
    C03_api_chat_roundtrip.run,
    C04_claude_chat_tools.run,
    C05_openai_api_chat_roundtrip.run,
]
