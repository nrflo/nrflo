# Agent Lifecycle Commands

Spawned agents report results via CLI. **Exiting with code 0 is an implicit
pass** — `agent finished` is the explicit equivalent. Only call `agent fail`
when the task cannot be completed. Context is provided automatically by the
system.

> **Note:** Script-mode agents (`execution_mode=script`) interact with
> lifecycle via the Python SDK instead of these CLI commands. See
> [python.md](python.md) for the SDK equivalents.

```bash
# Mark agent as successfully finished (proceed to next phase)
nrflo agent finished

# Mark agent as failed
nrflo agent fail [--reason <text>]

# Signal context exhaustion — triggers relaunch with fresh context
nrflo agent continue

# Callback to re-run earlier layers — exactly one flag required
nrflo agent callback --level <N>            # whole layer N
nrflo agent callback --agent <agent-id>     # single agent
nrflo agent callback --chain <a,b,...>      # sequential named agents

# Skip a workflow group tag
nrflo skip <tag>

# Workflow chain handoff — set data for the next step before finishing
nrflo agent chain-next-instructions --instructions "<text>"
nrflo agent chain-next-ticket --ticket-id "<id>"
```

| Command | When to use |
|---------|------------|
| `finished` | Task completed successfully; orchestrator advances to next phase |
| `fail` | Task cannot be completed; `--reason` is optional but recommended |
| `continue` | Context window exhausted; save progress to `to_resume` first |
| `callback` | Issue found requiring re-run; supply exactly one of `--level`, `--agent`, `--chain` |
| `skip <tag>` | Skip a workflow group in subsequent layers; tag must be in workflow's `groups` |
| `chain-next-instructions` | Pass instructions to the next chain step; call before `finished` |
| `chain-next-ticket` | Set the ticket ID for the next ticket-scope chain step; call before `finished` |

**Completion semantics:** Exit 0 or `agent finished` = pass. Non-zero exit or
`agent fail` = fail. `agent continue` triggers a fresh relaunch for
context-exhausted agents — it is not a success signal.

---

## Artifact CLI

Agents can upload, list, and retrieve artifacts at runtime. `$NRF_ARTIFACTS_DIR`
points to the pre-materialized stage directory for input artifacts.

```bash
# Upload a file as a named artifact (max 32 MiB)
nrflo agent artifact add <file-path> <NAME>

# List all artifacts for this workflow instance
nrflo agent artifact list

# Get the local abs path of a materialized artifact (cat-pipe friendly)
nrflo agent artifact get <NAME>
cat $(nrflo agent artifact get report.csv)
```

All commands read `NRF_SESSION_ID` and `NRF_WORKFLOW_INSTANCE_ID` from the
environment (set automatically by the spawner).

---

## Findings CLI

### Agent-Level Findings

```bash
# Add single finding (own session)
nrflo findings add <key> <value>

# Add multiple findings (batch syntax)
nrflo findings add key1:val1 key2:val2

# Append to existing finding (creates array if needed)
nrflo findings append <key> <value>
nrflo findings append key1:val1 key2:val2

# Get own findings
nrflo findings get                      # all own findings
nrflo findings get <key>               # single key (positional)
nrflo findings get -k key1 -k key2    # multiple keys

# Get another agent's findings (cross-agent read)
nrflo findings get <agent-type>             # all findings for agent
nrflo findings get <agent-type> <key>      # single key
nrflo findings get <agent-type> -k key1    # multiple keys
nrflo findings get --layer 1               # all findings for every agent at layer 1

# Delete findings
nrflo findings delete <key1> [key2...]
```

### Project-Level Findings

```bash
# Add
nrflo findings project-add <key> <value>
nrflo findings project-add key1:val1 key2:val2

# Get
nrflo findings project-get                    # all project findings
nrflo findings project-get <key>              # single key
nrflo findings project-get -k key1 -k key2    # multiple keys

# Append
nrflo findings project-append <key> <value>
nrflo findings project-append key1:val1 key2:val2

# Delete
nrflo findings project-delete <key1> [key2...]
```

### Batch Syntax

Both `add` and `append` support `key:value` pairs. The first colon separates
the key from the value:

```bash
nrflo findings add summary:'Fixed the auth bug' files_changed:'["auth.go"]'
```
