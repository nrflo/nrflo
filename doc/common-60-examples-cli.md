# Examples & Spec Import

---

## Common Patterns & Examples

### Example 1: Setup Analyzer Prompt

```markdown
You are a setup analyzer for ticket ${TICKET_ID}.

## Ticket
- **Title:** ${TICKET_TITLE}
- **Description:** ${TICKET_DESCRIPTION}

## Project Context
#{PROJECT_FINDINGS:architecture,conventions}

## Your Task

Analyze the ticket and codebase. Store your findings:

- `summary` — Brief analysis of what needs to be done
- `files_to_modify` — JSON array of file paths
- `implementation_plan` — Step-by-step plan

When done, save your findings (the `findings_add` tool, once per key) and exit
cleanly (exit 0 = pass): keys `summary`, `files_to_modify`, `implementation_plan`.
```

### Example 2: Implementor with Findings Injection and Callbacks

```markdown
Implement changes for ticket ${TICKET_ID} in the ${WORKFLOW} workflow.

## Previous Analysis
#{FINDINGS:setup-analyzer}

## Test Specifications
#{FINDINGS:test-writer:test_cases,coverage_plan}

## Your Task

Implement the changes described in the analysis. Follow the test specifications.

When done, save your findings with the `findings_add` tool (keys
`be_changes_summary`, `be_files_changed`) and exit cleanly (exit 0 = pass).

If blocked, call the `agent_fail` tool with a `reason`.
```

---

## Ticket & Dependency Management

A project-scoped agent (e.g. a ticket-creator) creates tickets with the
`ticket_create` and `ticket_add_dependency` tools — see
[Ticket tools](common-30-lifecycle-cli.md#ticket-tools). `ticket_create` returns
the new ticket's `ticket_id`; pass it to `ticket_add_dependency` to wire a
blocking dependency. Both act on the agent's own project. Tickets are also
managed through the **web UI** and the REST API
(`/api/v1/tickets`, `/api/v1/dependencies`); reading or mutating other ticket
fields from an agent still uses a project-scoped python tool (Settings → Python
Scripts, `kind=tool`) over that REST API.

---

## Spec Import

The **Import** page (`/import`) creates a ticket from an external source
without manual typing. The importer normalizes the source into a ticket title,
description, and agent instructions for review before committing.

### Supported Sources

| Source | How to use |
|--------|-----------|
| **GitHub Issue** | Search by keyword (optional `owner/repo`) or paste a full issue URL |
| **Jira Issue** | Search by keyword, or paste a bare key (`PROJ-123`) or full URL |
| **Markdown / Text** | Paste any raw markdown or plain text |

### Required Environment Variables

Configure in **Project Settings → Environment Variables** before using the
corresponding source:

| Source | Required variables |
|--------|-------------------|
| GitHub Issue | `GITHUB_TOKEN` (optional but avoids rate-limiting; omit for public repos) |
| Jira | `JIRA_BASE_URL`, `JIRA_EMAIL`, `JIRA_API_TOKEN` — all required |
| Markdown | None |

### Workflow

1. **Source** — Pick GitHub Issue, Jira Issue, or Markdown
2. **Input** — Provide the content (search/select or paste), then click **Normalize**
3. **Preview** — Review and edit the generated title, description, and agent
   instructions; choose a workflow; click **Create Ticket**

The system creates the ticket and immediately navigates to its detail page.
