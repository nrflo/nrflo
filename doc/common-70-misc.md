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
`$NRFLO_HOME/agent.sock` (override with `NRFLO_SOCKET`). The `nrflo` CLI
reads this env var automatically — no manual wiring is needed in agent prompts.

The socket uses a JSON-RPC line-delimited protocol. Supported methods:
`findings.*`, `project_findings.*`, `agent.fail/continue/callback/context_update`,
`workflow.skip`, `ws.broadcast`, `artifact.add/list/get`.

See [be/internal/socket/CLAUDE.md](../be/internal/socket/CLAUDE.md) for
protocol details.

---

## Doc Layout

The `doc/` folder contains four kinds of files served by the documentation UI:

- **`doc/common-*.md`** — Shared concepts served under the "Common" tab.
  The backend concatenates all files matching `doc/common*.md` in lexicographic
  (sorted) order. Numeric prefixes enforce reading order; gaps in the sequence
  allow future insertion without renaming.
- **`doc/cli.md`** — Served 1:1 under the "CLI" tab
  (`execution_mode=cli_interactive`)
- **`doc/python.md`** — Served 1:1 under the "Python Script" tab
  (`execution_mode=script`)
- **`doc/api.md`** — Served 1:1 under the "API" tab (`execution_mode=api`)

To update: edit the markdown files directly. Changes are picked up on next page
load. Keep each file under 300 lines. If a mode file ever needs splitting, use
the same numeric-prefix convention and coordinate with the backend glob
contract.

Focus documentation on what agent authors need to know. Implementation details
belong in `be/internal/*/CLAUDE.md`.
