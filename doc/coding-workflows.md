# Coding workflows

NRFLO coordinates Claude Code and Codex steps for implementation, testing,
verification, and documentation. Coding is one use case of the workflow engine.

## Start an engineering run

1. [Install NRFLO](../README.md#quick-start), create a project for your repository,
   and authenticate the CLI you intend to use on the server host.
2. Choose a bundled workflow or configure your own on **Workflows**.
3. Create a ticket describing the task and start its workflow from the UI.
4. Inspect findings and logs; use supported browser takeover for a CLI session
   when you need to steer it, then resume orchestration.

| Template | Stages |
|----------|--------|
| `feature` | Analysis → tests → implementation → QA → documentation |
| `bugfix` | Analysis → implementation → QA |
| `hotfix` | Implementation |
| `docs` | Analysis → documentation |
| `refactor` | Analysis → implementation → QA |

A verifier can write callback instructions and request earlier work again.
[Workflow control](common-40-workflow.md) describes callback targeting and caps;
[CLI authoring](cli.md) covers runtime configuration.

## Worktrees and delivery

Ticket-scoped execution supports isolated git worktrees when enabled. Successful
worktree runs merge into the configured default branch, with conflict-resolver
handling and manual fallback; pushing after merge is optional. Project-scoped
runs operate in the project root and leave changes there.

Claude autonomous CLI steps use PTY sessions. Codex autonomous steps use its
app-server integration; interactive user/takeover sessions use a PTY. Script
steps have no live takeover or context continuation. CLI authentication and
billing depend on the locally configured account and provider terms. API mode
uses NRFLO's [in-process agent runtime](api.md) and configured credentials.

The native `nrflo_server console` and [external MCP bridge](mcp-external.md)
provide terminal/client entry points. [Resilience](common-50-resilience.md)
describes context continuation, stalls, and restarts.
