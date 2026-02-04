# Findings Schema

Standard findings format for all nrworkflow agents across projects.

## Overview

Findings are structured data stored during each workflow phase. They enable:
- Context sharing between agents
- Progress tracking
- Audit trail of decisions

**Important:** Findings are isolated per workflow. Each workflow on a ticket maintains its own independent findings.

## Storage

Findings are stored via the nrworkflow CLI by **agent type**. The `--workflow/-w` flag is **required**:
```bash
# Add findings (two syntax modes)
nrworkflow findings add <ticket> <agent-type> <key> <value> -w <workflow>              # legacy: single key-value
nrworkflow findings add <ticket> <agent-type> key:'value' [key2:'value2'] -w <workflow>  # multiple key:value pairs

# Get findings
nrworkflow findings get <ticket> <agent-type> -w <workflow>              # all findings
nrworkflow findings get <ticket> <agent-type> -w <workflow> -k <key>     # specific key
nrworkflow findings get <ticket> <agent-type> -w <workflow> -k a -k b    # multiple keys
```

Values can be strings or JSON (arrays, objects).

### Model-Keyed Findings (v4)

For parallel agents, findings can be stored under a model identifier using `--model`:
```bash
# Store findings for specific model
nrworkflow findings add <ticket> <agent-type> <key> <value> -w <workflow> --model=claude:sonnet
nrworkflow findings add <ticket> <agent-type> key:'value' -w <workflow> --model=opencode:opus

# Get findings for specific model
nrworkflow findings get <ticket> <agent-type> -w <workflow> --model=claude:sonnet

# Get ALL parallel agents' findings grouped by model
nrworkflow findings get <ticket> <agent-type> -w <workflow>
# Returns: {"claude:sonnet": {...}, "opencode:opus": {...}}

# Get specific key from ALL parallel agents
nrworkflow findings get <ticket> <agent-type> -w <workflow> -k <key>
# Returns: {"claude:sonnet": "value1", "opencode:opus": "value2"}
```

#### Storage Structure

**Without --model (legacy):**
```json
{
  "setup-analyzer": {
    "summary": "...",
    "files_to_modify": [...]
  }
}
```

**With --model (parallel agents):**
```json
{
  "setup-analyzer": {
    "claude:sonnet": {
      "summary": "Analysis from Claude...",
      "files_to_modify": [...]
    },
    "opencode:opus": {
      "summary": "Analysis from OpenCode...",
      "files_to_modify": [...]
    }
  }
}
```

## Agent Schemas

### setup-analyzer

| Key | Type | Required | Description |
|-----|------|----------|-------------|
| summary | string | Yes | Brief summary of what needs to be done |
| acceptance_criteria | array | Yes | List of acceptance criteria from ticket |
| files_to_modify | array | Yes | Files that need changes (with paths) |
| patterns | array | No | Existing patterns to follow |
| existing_tests | array | No | Related test files that exist |
| category_justification | string | No | Why this category was chosen |

Example:
```bash
nrworkflow findings add TICKET-123 setup-analyzer summary "Add validation to user input form" -w feature
nrworkflow findings add TICKET-123 setup-analyzer acceptance_criteria '["Input validates on blur","Error messages display inline"]' -w feature
nrworkflow findings add TICKET-123 setup-analyzer files_to_modify '["src/components/Form.tsx","src/utils/validation.ts"]' -w feature
```

### test-writer

| Key | Type | Required | Description |
|-----|------|----------|-------------|
| test_files | array | Yes | Test files created (with paths) |
| test_cases | array | Yes | List of test case names |
| coverage_plan | string | No | What acceptance criteria each test covers |

### implementor

| Key | Type | Required | Description |
|-----|------|----------|-------------|
| files_created | array | Yes | New files created (with paths) |
| files_modified | array | Yes | Existing files modified (with paths) |
| build_result | string | Yes | "pass" or "fail" |
| test_result | string | Yes | "pass", "fail", or "skipped" |
| summary | string | No | Brief summary of changes made |

### qa-verifier

| Key | Type | Required | Description |
|-----|------|----------|-------------|
| verdict | string | Yes | "pass" or "fail" |
| criteria_status | object | Yes | Map of criterion to pass/fail with notes |
| issues | array | No | List of issues found (empty if pass) |
| test_result | string | Yes | "pass" or "fail" |
| notes | string | No | Any additional observations |

Example:
```bash
nrworkflow findings add TICKET-123 qa-verifier verdict "pass" -w feature
nrworkflow findings add TICKET-123 qa-verifier criteria_status '{"Input validates on blur":"pass","Error messages display inline":"pass"}' -w feature
```

### doc-updater

| Key | Type | Required | Description |
|-----|------|----------|-------------|
| docs_updated | array | Yes | Documentation files updated (with paths) |
| summary | string | No | Brief description of doc changes |

## Reading Findings

To read findings from another agent:
```bash
nrworkflow findings get <ticket> <agent-type> -w <workflow>
```

Returns JSON object with all keys stored by that agent for the specified workflow.

**Note:** Findings are isolated per workflow. If a ticket has both `feature` and `bugfix` workflows, each has its own findings.

## Workflow-Level Findings

Use `workflow` as the agent_type to store global findings that apply to the entire workflow rather than a specific agent phase.

### When to Use

- Data that needs to be shared across multiple agents
- Workflow-wide configuration or state
- Cross-phase metadata (e.g., selected architecture, global constraints)
- Information that doesn't belong to any specific agent

### Usage

```bash
# Store workflow-level finding
nrworkflow findings add <ticket> workflow <key> '<value>' -w <workflow>
nrworkflow findings add <ticket> workflow key:'value' [key2:'value2'] -w <workflow>

# Retrieve workflow-level findings
nrworkflow findings get <ticket> workflow -w <workflow>              # all
nrworkflow findings get <ticket> workflow -w <workflow> -k <key>     # specific key
```

### Example

```bash
# Store global configuration during investigation
nrworkflow findings add TICKET-123 workflow selected_architecture 'microservices' -w feature
nrworkflow findings add TICKET-123 workflow db_choice 'postgresql' -w feature

# Later agents can read these
nrworkflow findings get TICKET-123 workflow -w feature -k selected_architecture
# Returns: "microservices"
```

### Storage Structure

Workflow-level findings are stored alongside agent findings:

```json
{
  "findings": {
    "workflow": {
      "selected_architecture": "microservices",
      "db_choice": "postgresql"
    },
    "setup-analyzer": {
      "summary": "...",
      "files_to_modify": [...]
    }
  }
}
```

**Note:** `workflow` is a reserved agent_type. Do not create an actual agent named "workflow".

## Best Practices

1. **Store findings immediately** - Don't wait until the end
2. **Use JSON for arrays/objects** - Easier to parse later
3. **Be specific** - Include file paths, not just names
4. **Keep summaries brief** - One or two sentences max
5. **Validate before storing** - Ensure data is accurate
6. **Use workflow-level for shared data** - Cross-agent state goes in `workflow` findings
