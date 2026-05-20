# Workflow Result, Agent Fields, and Callbacks

---

## Workflow Result

Any agent can write a `workflow_final_result` finding to surface a
human-readable result summary after workflow completion.

```bash
nrflo findings add workflow_final_result:"Implementation complete: added auth middleware"
```

- Appears as a top-level field in the workflow state API response
- Displayed in the UI above the agent flow tree after workflow completion
- **Last-writer-wins:** the value from the session with the latest `ended_at` is used
- When notification channels (Slack, Telegram) are configured, the summary is
  included as a blockquote in completion notifications

---

## Agent Definition Fields

Configured via the agent form on the **Workflows** page.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `id` | string | Required | Agent type identifier (e.g., `setup-analyzer`, `implementor`) |
| `layer` | int | `0` | Phase execution layer (>=0). Same-layer agents run concurrently; layers execute in ascending order |
| `model` | string | `sonnet` | Model to use (see [cli.md](cli.md) for model table) |
| `timeout` | int | `20` | Max execution time in minutes |
| `prompt` | string | Required | Prompt template with `${VAR}` and `#{FINDINGS:...}` patterns |
| `restart_threshold` | int | `25` | Percentage of context remaining that triggers low-context save |
| `max_fail_restarts` | int | `0` | Max auto-restarts on failure (0 = disabled) |
| `stall_start_timeout_sec` | int | Configurable | Seconds with no output before start-stall restart; `0` = disabled |
| `stall_running_timeout_sec` | int | Configurable | Seconds of silence mid-execution before running-stall restart; `0` = disabled |
| `tag` | string | `""` | Group tag for skip-tag feature; must be in parent workflow's `groups` |
| `low_consumption_model` | string | `""` | Model override when low consumption mode is enabled globally |

Supported model values are in [cli.md](cli.md). The `model` field is ignored
for `execution_mode=script` agents.

---

## Workflow & Phase Configuration

### Phase Order

Phases are derived from agent definitions at read time, ordered by
`layer ASC, id ASC`. Example:

| Agent ID | Layer |
|----------|-------|
| setup-analyzer | 0 |
| test-writer | 1 |
| implementor | 2 |
| qa-verifier | 3 |

Execution order: setup-analyzer → test-writer → implementor → qa-verifier.

### Layer Execution Rules

- All agents in the same layer run concurrently
- Layers execute in ascending order (0, 1, 2, …)
- Multiple agents in layer N can feed into multiple agents in layer N+1
- If all agents in a layer are skipped, the workflow continues regardless of policy

### Layer Pass Policies

Each layer has a `pass_policy` (default: `any`). Skipped agents are excluded.

| Policy | Required passes |
|--------|----------------|
| `any` | At least 1 |
| `all` | All non-skipped agents |
| `quorum:N` | At least N |
| `percent:P` | `ceil(count × P / 100)` |

Set via `PUT /api/v1/workflows/{id}/layer-policies/{layer}` (admin only).

### Workflow Groups (Skip Tags)

Workflows define `groups` (e.g., `["be", "fe", "docs"]`). Agents are assigned
a `tag` from the workflow's groups. During execution, an agent can call
`nrflo skip <tag>` to add that tag to the instance's skip list; the
orchestrator checks skip tags before each layer.

### Scope Types

| Scope | Ticket required | Git worktree | Concurrent instances |
|-------|-----------------|--------------|---------------------|
| `ticket` | Yes | Yes | One per ticket+workflow |
| `project` | No | No (runs in project root) | Multiple allowed |

Project-level workflows run directly in the project root — automatic
merge-on-completion does not apply. File changes remain as uncommitted
changes in the project directory.

---

## Callback Mechanism

A later-layer agent (e.g., `qa-verifier`) can trigger re-execution of
earlier layers. Exactly one flag must be supplied:

| Flag | Effect |
|------|--------|
| `--level N` | Re-runs all agents in layer N, then every layer between N and the calling layer, then resumes forward |
| `--agent <id>` | Re-runs only the named agent, then re-runs every higher layer whole, then resumes forward |
| `--chain a,b,...` | Re-runs named agents sequentially; layers must be strictly ascending and the last ≤ calling layer |

### Instructions Placement

`${CALLBACK_INSTRUCTIONS}` is expanded for:
- **`--level`** — every agent in the target layer
- **`--agent`** — the named agent only
- **`--chain`** — the first agent in the chain only

### How to Trigger

```bash
# 1. Save callback instructions
nrflo findings add callback_instructions:"Fix the auth bug in middleware/auth.go"

# 2. Trigger with the desired mode
nrflo agent callback --level 2
nrflo agent callback --agent implementor
nrflo agent callback --chain implementor,test-writer
```

After the callback plan completes, `${CALLBACK_INSTRUCTIONS}` is cleared.

### Limits

- Maximum **10** cumulative agent spawns across all callback plan steps per workflow run
- `--agent` and `--chain` steps cannot themselves issue further callbacks (v1 restriction)
