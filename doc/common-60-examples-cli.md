# Examples, Ticket CLI, and Spec Import

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

When done, save your findings and exit cleanly (exit 0 = pass):
nrflo findings add summary:'...' files_to_modify:'[...]' implementation_plan:'...'
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

When done, save your findings and exit cleanly (exit 0 = pass):
nrflo findings add be_changes_summary:'...' be_files_changed:'[...]'

If blocked, fail with a reason:
nrflo agent fail --reason "..."
```

---

## Ticket Management CLI

Use the `nrflo tickets` CLI — **never use `curl` or direct HTTP API calls**.
Requires `NRFLO_PROJECT` env var (already set in spawned sessions).

```bash
# List tickets
nrflo tickets list
nrflo tickets list --status open --type task --parent EPIC-1

# Get a ticket
nrflo tickets get TICKET-1

# Create a ticket
nrflo tickets create --title "My task" [--id MY-ID] [--description "..."] \
  [--type task|bug|epic|story] [--priority 1-4] [--parent PARENT-ID]

# Update ticket fields (only specified flags are changed)
nrflo tickets update TICKET-1 --title "New title"
nrflo tickets update TICKET-1 --parent EPIC-1       # set parent
nrflo tickets update TICKET-1 --parent ""           # clear parent
nrflo tickets update TICKET-1 --priority 2 --type bug

# Close / reopen
nrflo tickets close TICKET-1 [--reason "Done"]
nrflo tickets reopen TICKET-1

# Dependency management
nrflo deps list TICKET-1
nrflo deps add TICKET-1 BLOCKER-1      # TICKET-1 is blocked by BLOCKER-1
nrflo deps remove TICKET-1 BLOCKER-1
```

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
