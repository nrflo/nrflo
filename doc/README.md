# NRFLO documentation

NRFLO is a self-hosted workflow execution engine for AI-powered applications.
Combine Python scripts and model-driven steps, pass structured findings, and
inspect execution through the web UI or application interfaces.

## Start here

1. [Install NRFLO](../README.md#quick-start) and create a project in the web UI.
2. [Run a mixed script + agent workflow](mixed-workflow.md).
3. [Call NRFLO from your application](application-integration.md).
4. Read [execution and security boundaries](execution-semantics.md).

## Concepts and authoring

| Topic | Reference |
|-------|-----------|
| Layers, fan-in, skips, callbacks, pause boundaries | [Workflow control](common-40-workflow.md) |
| Findings and explicit handoffs | [Findings](common-20-findings.md) |
| Scripts for deterministic processing | [Python mode and SDK](python.md) |
| Models for interpretation and reasoning | [API mode](api.md) |
| Provider CLI steps | [CLI authoring](cli.md) |
| Server-controlled step validation | [Stepwise execution](common-45-stepwise.md) |
| Failure and context handling | [Resilience](common-50-resilience.md) |
| Local models | [Local provider configuration](local-providers.md) |
| External MCP clients | [MCP bridge](mcp-external.md) |

## Use cases

- [Document intake](mixed-workflow.md): prepare text, extract fields, validate evidence.
- [TransferDue](transferdue.md): a verified first-party application integration.
- [Coding workflows](coding-workflows.md): worktrees, tests, QA, merge, takeover.

The installed server also serves execution-mode authoring documentation at
`/documentation`. This directory contains the onboarding and integration guides
as well as those authoring references.
