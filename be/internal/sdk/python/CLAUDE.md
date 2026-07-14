# be/internal/sdk/python

Single-file Python SDK shipped with the server binary. Used by `execution_mode=script` agents to talk to the server's Unix socket. Deep mechanics (full method table, notification payload, extension steps): [REFERENCE.md](REFERENCE.md) — read it before changing the SDK surface.

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

`nrflo_sdk.client()` exposes method groups (`c.findings`, `c.project_findings`, `c.agent`, `c.workflow`, `c.artifacts`) plus `c.context()` and its convenience accessors, `c.skip()`, `c.log()`, and `c.notification()`. Full method table + artifact size caps: [REFERENCE.md](REFERENCE.md#surface-area) — read before adding or changing SDK methods.

## Notification Payload

Notification scripts read a JSON payload from `NRFLO_NOTIFY_PAYLOAD_JSON` via `c.notification()` or module-level `nrflo_sdk.notification()` (no socket needed). Property map + error behaviour: [REFERENCE.md](REFERENCE.md#notification-payload).

## Adding to the SDK

New methods start at the socket handler, then a thin Python wrapper in `nrflo_sdk.py`, then the fake-server test harness. Steps: [REFERENCE.md](REFERENCE.md#adding-to-the-sdk) — read before extending the SDK.

`make test-pkg PKG=sdk/python` and `make test-pkg PKG=socket`.
