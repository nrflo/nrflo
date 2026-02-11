# Feature Workflow Definition

## Workflow (POST /api/v1/workflows)

```json
{
  "id": "feature",
  "description": "Full feature implementation: plan, implement, write tests, verify",
  "categories": ["full", "simple"],
  "phases": [
    {"agent": "planner"},
    {"agent": "implementor"},
    {"agent": "test-writer", "skip_for": ["simple"]},
    {"agent": "verifier"}
  ]
}
```

## Agent Definitions (POST /api/v1/workflows/feature/agents)

### planner

```json
{
  "id": "planner",
  "model": "opus",
  "timeout": 20,
  "prompt": "<contents of planner.md>"
}
```

### implementor

```json
{
  "id": "implementor",
  "model": "opus",
  "timeout": 40,
  "prompt": "<contents of implementor.md>"
}
```

### test-writer

```json
{
  "id": "test-writer",
  "model": "opus",
  "timeout": 30,
  "prompt": "<contents of test-writer.md>"
}
```

### verifier

```json
{
  "id": "verifier",
  "model": "opus",
  "timeout": 25,
  "prompt": "<contents of verifier.md>"
}
```

## Findings Flow

```
PLANNER (opus, 20min) ──read-only──→ findings
  plan_summary, files_to_modify, files_to_create,
  implementation_steps, patterns_to_follow, testing_notes, risks
  │
  ├──→ IMPLEMENTOR (opus, 40min) ──code──→ findings
  │      changes_summary, files_changed, testing_guidance, build_status
  │      │
  │      ├──→ TEST-WRITER (opus, 30min) ──tests──→ findings
  │      │      tests_written, test_run_status, coverage_notes, production_bugs
  │      │      │
  └──────┴──────┴──→ VERIFIER (opus, 25min) ──docs+commit──→ findings
                       build_status, test_status, docs_updated, bugs_fixed,
                       commit_hash, verification_notes
```

## Template Variables

Templates use these ticket context variables (fetched on demand by the spawner):
- `${TICKET_TITLE}` — from tickets table
- `${TICKET_DESCRIPTION}` — from tickets table
- `${USER_INSTRUCTIONS}` — from workflow_instances.findings["user_instructions"]
