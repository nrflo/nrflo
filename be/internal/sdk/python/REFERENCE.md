# Python SDK Reference

Deep mechanics for this package. The auto-loaded map lives in [CLAUDE.md](CLAUDE.md); read the relevant section here before changing the SDK surface.

Contents: [Surface Area](#surface-area) · [Notification Payload](#notification-payload) · [Adding to the SDK](#adding-to-the-sdk)

## Surface Area

| Group | Methods |
|-------|---------|
| `c.findings` | `add(key, value)`, `add_bulk(dict)`, `emit(key, value)`, `append(key, value)`, `append_bulk(dict)`, `get(agent_type=None, *, key=None, keys=None, layer=None)`, `delete(*keys)` — `layer=N` returns a flat `{agent_type: findings_dict\|None}` map; `agent_type` and `layer` are mutually exclusive. `emit` checks `value` against the key's schema; mismatch raises `NrfloError` |
| `c.project_findings` | Same shape as `c.findings` but scoped to project |
| `c.agent` | `finished()`, `fail(reason="")`, `continue_()`, `callback(level)`, `chain_next_ticket(ticket_id)`, `consult(consultant, question) -> str` |
| `c.workflow` | `continue_(instructions="", instance_id=None)`, `fail(reason, instance_id=None)` — continue/fail a workflow instance (defaults to current `_iid`) |
| `c.artifacts` | `add(name, content, content_type=None)`, `list()`, `get(name)` |
| `c.context(refresh=False)` | Cached call to the `script.context` socket method (19-key dict — see [be/internal/socket/CLAUDE.md](../../socket/CLAUDE.md)) |
| `c.seed_findings()` | Convenience: `c.context()["seed_findings"]` — caller-supplied `RunRequest.SeedFindings` keys (workflow_instance scope, excluding `user_instructions` and underscore-prefixed orchestrator-internal keys) |
| `c.user_instructions()` | Convenience: `c.context()["user_instructions"]` |
| `c.callback_info()` | Convenience: `c.context()["callback"]` (or `None`) |
| `c.previous_data()` | Convenience: `c.context()["previous_data"]` (set on relaunch via `to_resume`) |
| `c.workflow_result()` | Convenience: `c.context()["workflow_result"]` — `"pass"`, `"fail"`, or `""` |
| `c.workflow_status()` | Convenience: `c.context()["workflow_status"]` — raw instance status string |
| `c.workflow_final_result()` | Convenience: `c.context()["workflow_final_result"]` — session finding summary |
| `c.failure_reason()` | Convenience: `c.context()["failure_reason"]` — reason from `_failure_reason` finding |
| `c.external_id()` | Convenience: `c.context()["external_id"]` — `external_id` from the workflow instance ("" if unset) |
| `c.external_context()` | Convenience: `c.context()["external_context"]` — `external_context` from the workflow instance ("" if unset) |
| `c.skip(tag)` | Forwards to the `workflow.skip` socket method |
| `c.log(type, message, payload=None)` | Insert a message row via `agent.log`; no project required. `type` defaults to `"text"` — accepted values: `text`, `tool`, `subagent`, `skill`, `user_input`, `error`, `result`. `payload` is an optional Python value serialised to JSON. Output appears in the Logs UI Messages tab and server log. |
| `c.notification()` | Cached `_Notification` parsed from `NRFLO_NOTIFY_PAYLOAD_JSON`. Raises `NrfloError` if env var is missing or empty. No socket call. |

`c.artifacts.add()` accepts `str` (UTF-8 encoded) or `bytes`/`bytearray`, enforces a 32 MiB client-side cap (raises `NrfloError` before sending), and base64-encodes the payload as `content_b64`. Note: `$NRF_ARTIFACTS_DIR` is a read-only pre-staged fallback set at spawn time and does not reflect artifacts uploaded by sibling agents mid-run; use `c.artifacts.list()`/`get()` to access those.

## Notification Payload

Notification scripts receive a JSON payload in `NRFLO_NOTIFY_PAYLOAD_JSON`. Access it via:

- `c.notification()` — cached `_Notification` on a Client instance
- `nrflo_sdk.notification()` — module-level; no socket/client required (useful for pure notification scripts)

`_Notification` properties (all return `""` if key absent):

| Property | JSON key |
|----------|----------|
| `event_type` | `event_type` |
| `project_id` | `project_id` |
| `project_name` | `project_name` |
| `workflow` | `workflow` |
| `instance_id` | `instance_id` |
| `ticket_id` | `ticket_id` |
| `ticket_name` | `ticket_name` |
| `agent_type` | `agent_type` |
| `reason` | `reason` |
| `summary` | `workflow_final_result` |
| `raw` | full parsed dict |

Raises `NrfloError(0, "no notification payload in env …")` when `NRFLO_NOTIFY_PAYLOAD_JSON` is unset or empty.

Underlying `_Connection` class keeps a persistent Unix socket open and reconnects on broken pipe (up to 1s of retries). All errors map to `NrfloError(code, message)`.

## Adding to the SDK

1. Add the new method on the relevant socket handler (`be/internal/socket/handler*.go`) — see [be/internal/socket/CLAUDE.md](../../socket/CLAUDE.md).
2. Wire a thin Python wrapper in `nrflo_sdk.py` (call `_Connection.send(method, params)`).
3. Update `test_nrflo_sdk.py` to exercise the new method against the fake server in `sdk_test.go`.
4. Re-run `make test-pkg PKG=sdk/python` and `make test-pkg PKG=socket`.

The embed copy is auto-rebuilt — no manual `make` step is needed for SDK changes.
