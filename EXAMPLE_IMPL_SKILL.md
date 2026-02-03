---
name: impl
description: Orchestrate ticket implementation through investigation, TDD testing, implementation, and verification phases. Uses nrworkflow spawner for agent management.
---

# Impl Skill

Orchestrate ticket implementation using the nrworkflow agent spawner for state management.

## Workflow

```
1. Check state → 2. investigation → 3. test-design → 4. implementation → 5. verification → 6. docs → 7. close
```

**No questions allowed** - all clarifications resolved in /prep.

---

## Phase 1: Selection & State Check

1. Show ticket overview for context:
   ```bash
   nrworkflow ticket status  # Dashboard: pending + completed
   ```

2. If ticket ID provided as argument, use it. Otherwise:
   ```bash
   nrworkflow ticket ready   # Only unblocked tickets
   ```
   Use AskUserQuestion to let user select a ticket.

3. Check nrworkflow state:
   ```bash
   nrworkflow status <ticket>
   ```

4. Handle state:
   - **Not initialized**: Error - "Run /prep first"
   - **In progress**: Resume from current phase
   - **Completed**: "Ticket already implemented"

5. Claim the ticket:
   ```bash
   nrworkflow ticket update <ticket> --status=in_progress
   ```

---

## Phase 2: Investigation

Spawn setup-analyzer using the agent spawner:

```bash
nrworkflow agent spawn setup-analyzer <ticket-id> --session=$SESSION_MARKER -w feature
```

The spawner:
- Loads base template from `~/.nrworkflow/agents/setup-analyzer.base.md`
- Loads project override from `.claude/nrworkflow/overrides/setup-analyzer.md` (if exists)
- Injects ticket ID and session markers
- Manages phase start/stop automatically
- Reads agent result from ticket state (set by `nrworkflow agent complete/fail`)

**Category is set by the agent** via:
```bash
nrworkflow set <ticket> category <docs|simple|full>
```

**Category rules:**
- **docs**: Only markdown/text files, no code logic
- **simple**: Code change with existing test coverage
- **full**: New feature or behavior requiring TDD

---

## Phase 3: Test Design (auto-skipped for docs/simple)

The spawner automatically handles skip_for based on category:

```bash
nrworkflow agent spawn test-writer <ticket-id> --session=$SESSION_MARKER -w feature
```

If category is `docs` or `simple`, the spawner will:
- Skip the phase automatically
- Mark phase as `skipped`
- Return success (exit code 0)

---

## Phase 4: Implementation

```bash
nrworkflow agent spawn implementor <ticket-id> --session=$SESSION_MARKER -w feature
```

The implementor agent:
- Reads findings from setup-analyzer and test-writer agents
- Implements the ticket following patterns
- Stores findings under its agent type (implementor)
- Calls `nrworkflow agent complete/fail` when done

---

## Phase 5: Verification (auto-skipped for docs)

```bash
nrworkflow agent spawn qa-verifier <ticket-id> --session=$SESSION_MARKER -w feature
```

If category is `docs`, the spawner auto-skips this phase.

**On FAIL**: Retry implementation once:
```bash
nrworkflow agent retry <ticket> -w feature
nrworkflow agent spawn implementor <ticket-id> --session=$SESSION_MARKER -w feature
```

---

## Phase 6: Documentation

```bash
nrworkflow agent spawn doc-updater <ticket-id> --session=$SESSION_MARKER -w feature
```

Updates project documentation based on implementation findings.

---

## Phase 7: Completion

1. Check final status:
   ```bash
   nrworkflow status <ticket>
   ```

2. Close and commit:
   ```bash
   nrworkflow ticket close <ticket> --reason="<summary>"
   git add <code-files>
   git commit -m "feat: <description> (<ticket>)"
   git push
   ```

3. Report to user:
   - Files changed
   - Tests added (if applicable)
   - Acceptance criteria verified

---

## Progress Tracking

Check progress at any time:
```bash
nrworkflow progress <ticket>
```

Shows:
- Current phase
- Active agent (if running)
- Completed agents with duration
- Phase status

---

## Error Handling

| Error | Action |
|-------|--------|
| Not initialized | "Run /prep first" |
| Agent stuck | `nrworkflow agent kill <ticket>`, then retry |
| Verification fail | Retry implementation once |
| Max retries exceeded | Stop, report to user |

---

## Agent Spawner Commands

```bash
# Spawn agent with full state management
nrworkflow agent spawn <agent_type> <ticket> --session=$SESSION_MARKER -w feature

# Preview assembled prompt (debugging)
nrworkflow agent preview <agent_type> <ticket> -w feature

# List available agents
nrworkflow agent list

# Kill stuck agent (uses PID)
nrworkflow agent kill <ticket> -w feature

# Retry failed agent
nrworkflow agent retry <ticket> -w feature
```

The spawner handles:
1. Phase validation (can't spawn out of order)
2. Category-based skip_for rules
3. Prompt assembly (base + override + variables)
4. State management (phase start/stop, agent tracking)
5. PID tracking for kill support
6. Agent result verification from ticket state

If agent times out or fails:
```bash
nrworkflow agent retry <ticket> -w feature
nrworkflow agent spawn <agent_type> <ticket> --session=$SESSION_MARKER -w feature
```
