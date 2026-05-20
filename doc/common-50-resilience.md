# Resilience: Low-Context, Failure Restart, Stall Detection

---

## Low-Context Continuation

When an agent's remaining context drops below the configured threshold, the
system automatically saves progress and relaunches with a fresh context window.

> **Note:** This mechanism does not apply to `execution_mode=script` agents.
> Script agents run to completion without context tracking.

### How It Works

1. System detects context usage exceeds threshold (default: 75% used, 25% remaining)
2. Kills the running agent and resumes the session with instructions to save
   progress to the `to_resume` finding
3. Agent calls `nrflo agent continue` after saving (`c.agent.continue_()` in Python SDK)
4. System launches a fresh agent with `${PREVIOUS_DATA}` populated from `to_resume`

### Configuration

`restart_threshold` in agent definition: percentage of context **remaining**
that triggers save (default `25`). Lower values = more aggressive (agent uses
more context before save).

### Agent Template Pattern

```markdown
## Previous Progress
${PREVIOUS_DATA}

## Your Task
Continue implementation from where the previous session left off.
```

---

## Automatic Failure Restart

When `max_fail_restarts > 0`, a failed agent is automatically restarted up to
`max_fail_restarts` times before the failure is final. Unlike low-context
continuation, the agent starts completely fresh — no `${PREVIOUS_DATA}`.

### How It Works

1. Agent exits with non-zero code or calls `agent fail`
2. System checks remaining restart budget
3. If restarts remain: relaunches fresh
4. If exhausted: failure propagates normally

Failure restarts are tracked independently from low-context restarts, so both
mechanisms can coexist on the same agent.

---

## Stall Detection & Auto-Restart

The system monitors agent output and automatically restarts frozen agents.

- **Start stall**: Agent produces no output within the start timeout
  (`stall_start_timeout_sec` on the agent definition)
- **Running stall**: Agent was active but stopped producing output for the
  running timeout (`stall_running_timeout_sec` on the agent definition)

### How It Works

1. System monitors time since last agent output
2. On stall: kills agent immediately (no context save) and relaunches fresh
3. Stall restarts are capped at 15 to prevent infinite loops

### Configuration

- `stall_start_timeout_sec`: `0` = disabled; `NULL` = use global default (120 s)
- `stall_running_timeout_sec`: `0` = disabled; `NULL` = use global default (480 s)
- Per-agent definition takes priority over global config, which takes priority
  over hardcoded defaults
- Stall restarts are tracked independently from failure and low-context restarts

For depth on global config keys and the broadcast/SIGTERM sequence, see
[be/internal/spawner/CLAUDE.md](../be/internal/spawner/CLAUDE.md).
