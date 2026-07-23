# Stepwise Agent Definitions

---

An agent definition with `prompt_mode="stepwise"` runs a fixed sequence of
steps instead of one open-ended prompt. The server holds a durable cursor
(`agent_step_cursors`, keyed by workflow instance + node) that survives
relaunches, and the agent advances it one step at a time by calling the
`complete_step` tool — the agent never advances the cursor itself.

## Step Definition Schema

`agent_definitions.steps` is a JSON array of up to 20 step objects:

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| `step_id` | string | Yes | Unique per def; must match `^[a-z0-9][a-z0-9_-]{0,63}$` |
| `title` | string | Yes | Non-empty |
| `instruction` | string | Yes | Non-empty, ≤16384 bytes |
| `required_findings` | array | No | ≤20 entries: `{key, schema}`; `key` non-empty, whitespace-free, ≤128 bytes; `schema` one of the named finding schemas below |
| `checks` | array of string | No | ≤20 entries, ≤1024 bytes each, non-empty; shell commands run after evidence passes, same executor as `validation_commands` |
| `rotation_allowed` | bool | No | Whether the orchestrator may rotate the assigned model/session when this step stalls; ignored on the final step |
| `path_overlap` | object | No | `{left: [...], right: [...]}` — cross-key gate: the two groups of `required_findings` keys must claim no path in common (≤10 keys per side, no key in both) |

`prompt_mode="full"` forbids `steps`; `"stepwise"` requires at least one step
and is incompatible with `execution_mode="script"`.

## Named Finding Schemas

`required_findings[].schema` must be one of:

- **`nonempty_text`** — value decodes to a JSON string that is non-empty
  after trimming whitespace.
- **`ordered_lines`** — value decodes to a string with at least 2 non-empty
  lines, each matching `N. text` or `N) text`, numbers strictly ascending
  starting at 1.
- **`json_array_path_change`** — value decodes to a JSON array (may be
  empty); each element is an object with a non-empty string `path` plus at
  least one other non-empty descriptive string field (e.g. `change` or
  `purpose`). Every element's `path` feeds the `path_overlap` gate.

## Completing a Step

Call the `complete_step` tool once the current step's `required_findings`
have been recorded via `findings_add` and any `path_overlap` gate is clear.
The tool rejects with the specific missing/invalid key on failure (subject to
a per-step rejection cap before the session is force-failed) or advances the
cursor to the next step, rotates the session, or reports the sequence done.
An agent whose session ends with the cursor short of its last step is
force-failed regardless of its own reported result.
