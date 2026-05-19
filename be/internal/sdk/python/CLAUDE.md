# be/internal/sdk/python

Single-file Python SDK shipped with the server binary. Used by `execution_mode=script` agents to talk to the server's Unix socket.

## Files

| File | Purpose |
|------|---------|
| `nrflo_sdk.py` | Pure-stdlib SDK module (no external deps) |
| `embed.go` | `//go:embed nrflo_sdk.py` + `WriteSDK(dir)` installer; package `pythonsdk` |
| `embed_test.go` | Verifies the embedded copy round-trips through `WriteSDK` |
| `sdk_test.go` | Spins up the real SDK against a fake socket server |
| `test_nrflo_sdk.py` | Pure-Python harness exercised by `sdk_test.go` via `python3 -m unittest` |

## Install Flow

`pythonsdk.WriteSDK(sdkDir)` is called once on every `nrflo_server serve` startup (best-effort; WARN logged on failure). It writes `nrflo_sdk.py` into `<sdkDir>/nrflo_sdk.py` with mode `0o644`. Default `sdkDir` is `$NRFLO_HOME/sdk` (`~/.nrflo/sdk`).

The spawner exports `NRFLO_SDK_DIR=<sdkDir>` to script-mode agent processes; scripts bootstrap with:

```python
import os, sys
sys.path.insert(0, os.environ["NRFLO_SDK_DIR"])
import nrflo_sdk
c = nrflo_sdk.client()
```

## Identity Env Vars

`client()` reads `NRF_SESSION_ID`, `NRF_WORKFLOW_INSTANCE_ID`, `NRFLO_PROJECT`, `NRF_TRX` (all set by the spawner). Socket path: `$NRFLO_HOME/agent.sock` (fallback `~/.nrflo/agent.sock`); override via `NRFLO_SOCKET`.

## Surface Area

| Group | Methods |
|-------|---------|
| `c.findings` | `add(key, value)`, `add_bulk(dict)`, `append(key, value)`, `append_bulk(dict)`, `get(agent_type=None, *, key=None, keys=None, layer=None)`, `delete(*keys)` — `layer=N` returns a flat `{agent_type: findings_dict\|None}` map; `agent_type` and `layer` are mutually exclusive |
| `c.project_findings` | Same shape as `c.findings` but scoped to project |
| `c.agent` | `finished()`, `fail(reason="")`, `continue_()`, `callback(level)`, `chain_next_ticket(ticket_id)` |
| `c.artifacts` | `add(name, content, content_type=None)`, `list()`, `get(name)` |
| `c.context(refresh=False)` | Cached call to the `script.context` socket method (13-key dict — see [be/internal/socket/CLAUDE.md](../../socket/CLAUDE.md)) |
| `c.seed_findings()` | Convenience: `c.context()["seed_findings"]` — caller-supplied `RunRequest.SeedFindings` keys (workflow_instance scope, excluding `user_instructions` and underscore-prefixed orchestrator-internal keys) |
| `c.user_instructions()` | Convenience: `c.context()["user_instructions"]` |
| `c.callback_info()` | Convenience: `c.context()["callback"]` (or `None`) |
| `c.previous_data()` | Convenience: `c.context()["previous_data"]` (set on relaunch via `to_resume`) |
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
