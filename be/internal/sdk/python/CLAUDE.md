# be/internal/sdk/python

Single-file Python SDK shipped with the server binary. Used by `execution_mode='script'` agents (root CLAUDE.md principle 46) to talk to the server's Unix socket.

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

## Auto-Populated Identity

The `client()` constructor reads four env vars set by the spawner:

| Env var | Purpose |
|---------|---------|
| `NRF_SESSION_ID` | Current agent session — sent in every socket call |
| `NRF_WORKFLOW_INSTANCE_ID` | Workflow instance — sent in findings/agent calls |
| `NRFLO_PROJECT` | Project ID — derived for project-scoped findings |
| `NRF_TRX` | Trace ID propagated to server logs |

Socket path defaults to `/tmp/nrflo/nrflo.sock`; override via `NRFLO_SOCK_PATH`.

## Surface Area

| Group | Methods |
|-------|---------|
| `c.findings` | `add(key, value)`, `add_bulk(dict)`, `append(key, value)`, `append_bulk(dict)`, `get(agent=None, keys=None)`, `delete(*keys)` |
| `c.project_findings` | Same shape as `c.findings` but scoped to project |
| `c.agent` | `finished()`, `fail(reason="")`, `continue_()`, `callback(level)` |
| `c.context(refresh=False)` | Cached call to the `script.context` socket method (12-key dict — see [be/internal/socket/CLAUDE.md](../../socket/CLAUDE.md)) |
| `c.user_instructions()` | Convenience: `c.context()["user_instructions"]` |
| `c.callback_info()` | Convenience: `c.context()["callback"]` (or `None`) |
| `c.previous_data()` | Convenience: `c.context()["previous_data"]` (set on relaunch via `to_resume`) |
| `c.skip(tag)` | Forwards to the `workflow.skip` socket method |

Underlying `_Connection` class keeps a persistent Unix socket open and reconnects on broken pipe (up to 1s of retries). All errors map to `NrfloError(code, message)`.

## Adding to the SDK

1. Add the new method on the relevant socket handler (`be/internal/socket/handler*.go`) — see [be/internal/socket/CLAUDE.md](../../socket/CLAUDE.md).
2. Wire a thin Python wrapper in `nrflo_sdk.py` (call `_Connection.send(method, params)`).
3. Update `test_nrflo_sdk.py` to exercise the new method against the fake server in `sdk_test.go`.
4. Re-run `make test-pkg PKG=sdk/python` and `make test-pkg PKG=socket`.

The embed copy is auto-rebuilt — no manual `make` step is needed for SDK changes.
