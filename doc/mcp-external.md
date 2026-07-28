# External MCP Access

`nrflo_server agent mcp-external` is a stdio MCP bridge that lets any MCP
client — Claude Code, Codex CLI, an IDE, or a custom agent — drive a running
nrflo server. It proxies MCP `tools/list` / `tools/call` to the server's
console tool catalogue over HTTP; the bridge itself contains zero per-tool
code, so the tools you see are exactly what the server serves.

---

## Prerequisites

- A running `nrflo_server` (local or remote).
- A **service token**, minted in **Settings → Administration → Service
  Tokens**. A *global* token can open a session for any project; a
  *project* token only for its own project (anything else is rejected).

---

## Configuration

The bridge is configured entirely via environment variables:

| Variable | Required | Meaning |
|----------|----------|---------|
| `NRFLO_MCP_TOKEN` | yes | Service token, sent as `Authorization: Bearer` |
| `NRFLO_SERVER_URL` | no | Server base URL (default `http://127.0.0.1:6587`) |
| `NRFLO_PROJECT` | no | Explicit project id; see project resolution below |

### Project resolution

The project is resolved **once, at connect**, and stays fixed for the life of
the connection (tool schemas carry no `project` argument):

1. **Working-directory auto-detect** — the bridge's cwd is matched
   longest-prefix against the registered projects' root paths.
2. **`NRFLO_PROJECT`** — used when auto-detect finds nothing.
3. **Global project** — the hidden global scope; connecting never fails on
   project resolution.

The bridge also passes your current git branch as a current-ticket hint: when
the branch name matches a ticket in the resolved project, tools such as
`ticket_current` pick it up automatically.

---

## Connecting

Claude Code:

```bash
claude mcp add nrflo \
  --env NRFLO_MCP_TOKEN=<service token> \
  -- nrflo_server agent mcp-external
```

Generic MCP client configuration (stdio transport):

```json
{
  "mcpServers": {
    "nrflo": {
      "command": "nrflo_server",
      "args": ["agent", "mcp-external"],
      "env": {
        "NRFLO_MCP_TOKEN": "<service token>",
        "NRFLO_SERVER_URL": "http://127.0.0.1:6587"
      }
    }
  }
}
```

---

## Available tools

The catalogue is **server-owned** (the console tool profile): tickets,
findings, workflow lifecycle, artifacts, delegation, web search/fetch, and
more. New tools added server-side appear on the next `tools/list` — no
client or bridge update needed.

Not served: the plan-lifecycle builtins (`dynamic_workflow`, `revise_plan`,
`approve_plan`). Those are owned by a running workflow's parent instance;
drive them from the web UI or the REST plan routes instead.

---

## Session lifecycle

- Each connection opens one console session on the server and closes it on
  disconnect (stdio EOF).
- Idle sessions are swept server-side (`console_idle_ttl_hours`, default 12).
  If the session expires mid-connection, the bridge transparently exchanges a
  fresh one and retries the call once.
- Tool calls can legitimately run for minutes (e.g. starting and waiting on a
  workflow). The bridge sets no HTTP timeout of its own — raise your MCP
  client's tool-call timeout if it defaults to something short (for Claude
  Code, `MCP_TIMEOUT`).
- Cancelling a call in the client (`notifications/cancelled`) cancels the
  in-flight server request; there is no orphaned work on the server side.
