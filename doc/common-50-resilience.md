# Resilience: Low-Context, Failure Restart, Stall Detection

---

## Low-Context Continuation

When an agent's remaining context drops below the configured threshold, the
system automatically saves progress and relaunches with a fresh context window.

> **Note:** This mechanism does not apply to `execution_mode=script` agents.
> Script agents run to completion without context tracking.

### How It Works

1. The system stops the agent and flushes pending messages.
2. Full-prompt agents use a fresh working-set digest when available; otherwise a
   context-saver agent summarizes recorded history into `to_resume`.
3. Stepwise agents use the server-owned cursor as their continuation state.
4. The replacement session receives available continuation data automatically.
   Script steps have no context-window continuation.

### Configuration

`restart_threshold` in agent definition: percentage of context **remaining**
that triggers save (default `25`). Lower values = more aggressive (agent uses
more context before save).

Continuation instructions are prepended by the server's configured injectables.
Do not add `${PREVIOUS_DATA}` to an agent prompt: it is stripped there. See
[template variables](common-10-intro.md#auto-prepended-blocks).

---

## Automatic Failure Restart

When `max_fail_restarts > 0`, a failed agent is automatically restarted up to
`max_fail_restarts` times before the failure is final. Available continuation
findings and fresh working-set data may be carried into the replacement session;
this does not guarantee that every in-flight action was saved.

### How It Works

1. Agent exits with non-zero code or calls `agent_fail`
2. System checks remaining restart budget
3. If restarts remain: relaunches with available continuation data and
   applicable failure feedback
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
2. On stall: stops the agent without a new context-save attempt and relaunches
   with available continuation data
3. Stall restarts are capped at 15; three consecutive starts with no activity
   fail as `prompt_not_accepted`

### Configuration

- `stall_start_timeout_sec`: `0` = disabled; `NULL` = use global default (120 s)
- `stall_running_timeout_sec`: `0` = disabled; `NULL` = use global default (480 s)
- Per-agent definition takes priority over global config, which takes priority
  over hardcoded defaults
- Stall restarts are tracked independently from failure and low-context restarts

For depth on global config keys and the broadcast/SIGTERM sequence, see
[be/internal/spawner/CLAUDE.md](../be/internal/spawner/CLAUDE.md).

Persistent state does not imply crash resumption. See
[execution boundaries](execution-semantics.md#state-restarts-and-repeated-work)
for server restarts and external side-effect reconciliation.
