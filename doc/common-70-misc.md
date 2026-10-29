# System Agents, IPC Socket, and Doc Layout

---

## System Agents

System agents are global agent definitions not tied to any specific project or
workflow. They are managed on the **Settings** page. System agents are used for
system-level tasks such as automatic merge conflict resolution and context-save
summarization.

---

## Agent IPC Socket

Spawned agents communicate with the server over a Unix socket at
`$NRFLO_HOME/agent.sock` (override with `NRFLO_SOCKET`). The `agent mcp`
bridge and the other `nrflo_server agent` subcommands read this env var
automatically — no manual wiring is needed in agent prompts.

The socket uses a JSON-RPC line-delimited protocol. Supported methods:
`findings.*`, `project_findings.*`, agent lifecycle/context methods,
`workflow.skip`, `ws.broadcast`, `artifact.add/list/get`.

See [be/internal/socket/CLAUDE.md](../be/internal/socket/CLAUDE.md) for
protocol details.

---

## Doc Layout

The `doc/` folder contains onboarding/integration guides as well as the
execution-mode references served by the documentation UI:

- **`doc/common-*.md`** — Shared concepts served under the "Common" tab.
  The backend concatenates all files matching `doc/common*.md` in lexicographic
  (sorted) order. Numeric prefixes enforce reading order; gaps in the sequence
  allow future insertion without renaming.
- **`doc/cli.md`** — Served 1:1 under the "CLI" tab
  (`execution_mode=cli_interactive`)
- **`doc/python.md`** — Served 1:1 under the "Python Script" tab
  (`execution_mode=script`)
- **`doc/api.md`** — Served 1:1 under the "API" tab (`execution_mode=api`)
- **`doc/local-providers.md`** — Served 1:1 under the "Local Providers" tab
- **`doc/mcp-external.md`** — Served 1:1 under the "External MCP" tab
  (connecting external MCP clients via `nrflo_server agent mcp-external`)

Edit the source Markdown files, rebuild the UI assets and server binary, and
restart the server to publish embedded changes. Refreshing the page alone does
not reload source files from disk. Keep each file under 300 lines; the numeric
concatenation convention currently applies only to Common docs.

Start with the [documentation index](README.md) for application integration,
the mixed-workflow tutorial, and execution boundaries. Implementation details
belong in `be/internal/*/CLAUDE.md`.
