# Provider Capability Matrix

Source of truth for provider capability claims. If you add or remove a capability, update this file and the linked package CLAUDE.md.

| Provider | cli | cli_interactive | api | script | take-control | plan mode | resume-session | real-time context telemetry | rate-limit events | low-consumption mode | MCP tools (api-mode) | script-mode |
|----------|-----|-----------------|-----|--------|--------------|-----------|----------------|-----------------------------|-------------------|----------------------|----------------------|-------------|
| claude | ✅ | ✅ | ✅ | N/A | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | N/A |
| codex | ✅ | ✅ | ❌ | N/A | ✅ | ❌ | ✅ | ✅ | ✅ | ✅ | ❌ | N/A |
| opencode | ✅ | ❌ | ❌ | N/A | ❌ | ❌ | ❌ | ✅ | ❌ | ✅ | ❌ | N/A |
| gemini [planned] | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ |

Sources: `cli_adapter_claude.go:77` (claude SupportsInteractive), `cli_adapter_codex.go:91` (codex SupportsInteractive), `cli_adapter_opencode.go:129` (opencode SupportsInteractive=false), `cli_adapter_opencode.go:118` (opencode resume-session ❌), `cli_adapter_codex_jsonl_tail.go` (codex telemetry), `cli_adapter_opencode_sqlite_tail.go` (opencode telemetry), `spawner/apirun/CLAUDE.md` (claude api-mode + MCP tools), `manifest/CLAUDE.md` (MCP tools api-mode only).

[planned] Gemini adapter is not yet implemented; row listed for visibility only. Once shipped, flip cells in the same commit as the adapter.
