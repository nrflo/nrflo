# Python Script Mode (`execution_mode=script`)

For shared concepts (findings patterns, workflow config, callback mechanism,
examples), see the **Common** tab. Lifecycle and findings interact through the
`nrflo_sdk` Python module described here rather than CLI commands.

---

## What It Is

A stored Python script (pure stdlib + `nrflo_sdk`) executes as an agent step.
This is useful for deterministic logic, external API calls, data transforms, or
any task that should not be handed to an LLM.

- Exit 0 (or `c.agent.finished()`) = pass. Non-zero exit (or `c.agent.fail(...)`) = fail
- Full access to findings, project findings, and workflow context via the SDK
- No prompt template, no model selection, no context window — runs start to finish every time
- No low-context restart, no resume, and no take-control

---

## Authoring

Go to **Python Scripts** (`/python-scripts`) in the web UI. Create a new script
by giving it a name and pasting your Python code. The **Validate** button
syntax-checks the code via `python3` without saving.

Minimal script structure:

```python
import sys, os
sys.path.insert(0, os.environ["NRFLO_SDK_DIR"])
import nrflo_sdk

c = nrflo_sdk.client()

ctx = c.context()
ticket_id = ctx["ticket_id"]

files = c.findings.get("setup-analyzer", "files_to_modify")
c.findings.add("result", "all checks passed")

c.agent.finished()
```

---

## Wiring

On the **Workflows** page, add or edit an agent definition and set
**Execution Mode** to `Python Script`. A dropdown appears to pick one of your
saved scripts. The `model` field is ignored for script agents.

---

## Lifecycle

1. The spawner writes the script to `/tmp/nrflo/scripts/<session-id>.py`
2. Runs `python3 <path>` inside the agent's working directory (git worktree for
   ticket-scope, project root for project-scope)
3. Stdout lines are captured and shown in the agent message timeline
4. Stderr lines are shown as warnings in the timeline
5. Exit 0 = pass, non-zero = fail

---

## Python Dependencies (`requirements.txt`)

If your project root contains a `requirements.txt`, nrflo automatically creates
and maintains a per-project Python virtual environment before each workflow run.
The venv is stored at `$NRFLO_HOME/project/<projectID>/venv` and kept in sync
with `requirements.txt` (re-installed only when the file changes). Script
agents run inside this venv automatically.

If `requirements.txt` is absent or the venv cannot be created, agents fall back
to the system `python3` on PATH.

---

## nrflo_sdk Reference

```python
import sys, os
sys.path.insert(0, os.environ["NRFLO_SDK_DIR"])
import nrflo_sdk
c = nrflo_sdk.client()
```

The socket path is resolved automatically (see Agent IPC Socket in **Common**
for `NRFLO_SOCKET` override details).

### Agent Control

| Method | Description |
|--------|-------------|
| `c.agent.finished()` | Signal success and exit |
| `c.agent.fail(reason="")` | Signal failure and exit |
| `c.agent.continue_()` | Signal context exhaustion (triggers relaunch; rarely needed) |
| `c.agent.callback(level)` | Trigger callback to re-run an earlier layer |

### Own Findings

| Method | Description |
|--------|-------------|
| `c.findings.add(key, value)` | Set a finding |
| `c.findings.add_bulk({key: value, ...})` | Set multiple findings |
| `c.findings.get(key=None)` | Get own findings (all or by key) |
| `c.findings.append(key, value)` | Append to a finding (creates array) |
| `c.findings.append_bulk({...})` | Append multiple |
| `c.findings.delete(*keys)` | Delete findings |

Cross-agent read — pass agent type as first arg to `.get()`:

```python
c.findings.get("setup-analyzer", "files_to_modify")
```

### Project Findings

| Method | Description |
|--------|-------------|
| `c.project_findings.add(key, value)` | Set project-level finding |
| `c.project_findings.add_bulk({...})` | Set multiple |
| `c.project_findings.get(key=None)` | Get project findings |
| `c.project_findings.append(key, value)` | Append to project finding |
| `c.project_findings.append_bulk({...})` | Append multiple |
| `c.project_findings.delete(*keys)` | Delete |

### Workflow Context

| Method | Description |
|--------|-------------|
| `c.context(refresh=False)` | Return 12-key dict (cached; pass `refresh=True` to refetch) |
| `c.user_instructions()` | Return user-supplied instructions string ("" if none) |
| `c.callback_info()` | Return callback dict or `None` |
| `c.previous_data()` | Return `to_resume` string from a prior session ("" if none) |
| `c.skip(tag)` | Add a skip tag to the workflow instance |
| `c.log(type, message, payload=None)` | Insert a message row visible in the Logs UI Messages tab |

**`c.log()` accepted types:** `text` (default), `tool`, `subagent`, `skill`,
`user_input`, `error`, `result`. `payload` is any Python value (JSON-serialised).

### Context Dict Keys (`c.context()`)

| Key | Description |
|-----|-------------|
| `session_id` | This agent's session UUID |
| `instance_id` | Workflow instance UUID |
| `project_id` | Project identifier |
| `agent_type` | Agent type identifier |
| `workflow_id` | Workflow definition UUID |
| `scope_type` | `"ticket"` or `"project"` |
| `ticket_id` | Ticket ID (empty string for project-scope) |
| `ticket_title` | Ticket title (empty string for project-scope) |
| `ticket_description` | Ticket description (empty string for project-scope) |
| `user_instructions` | User-supplied instructions (empty string if none) |
| `callback` | `None` or `{"instructions": "...", "from_agent": "...", "level": N}` |
| `previous_data` | Content of `to_resume` finding from prior session ("" if none) |

---

## Errors

The SDK raises `nrflo_sdk.NrfloError(code, message)` for socket errors.
SDK calls retry with exponential backoff up to ~1 second before raising.
Unhandled exceptions cause a non-zero exit, marking the agent layer as failed.

---

## Worked Example: Gate That Fails When No Files Were Found

```python
import sys, os, json
sys.path.insert(0, os.environ["NRFLO_SDK_DIR"])
import nrflo_sdk

c = nrflo_sdk.client()

files_raw = c.findings.get("setup-analyzer", "files_to_modify")
if not files_raw:
    c.agent.fail(reason="setup-analyzer did not set files_to_modify")

try:
    files = json.loads(files_raw)
except Exception:
    c.agent.fail(reason="files_to_modify is not valid JSON")

if not files:
    c.agent.fail(reason="files_to_modify is empty — nothing to implement")

c.findings.add("validated_files", files_raw)
c.agent.finished()
```
