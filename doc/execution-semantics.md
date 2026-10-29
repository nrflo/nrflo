# Execution and deployment boundaries

NRFLO runs on one self-hosted server with a Go process, embedded web UI, and
SQLite state. Same-layer steps run concurrently on that host; configured
layers, fan-in policies, validation, and pause boundaries govern progression.
LLM outputs remain nondeterministic.

## State, restarts, and repeated work

Workflow instances, sessions, findings, and history persist in the database.
On server startup, an orphan sweep marks leftover in-flight work failed;
it does not transparently resume arbitrary Python processes or model requests.
Operators can inspect the failure and use supported retry/restart controls.

Failed-layer retries, configured failure restarts, and verifier callbacks can
execute operations again. An interrupted API call may have committed an
external action even when NRFLO did not record success. The application must
own idempotency keys, reconciliation, and transaction boundaries for payments,
messages, or other business actions. `external_id` is correlation metadata.

This is a single-server architecture. Distributed workers, cluster failover,
and exactly-once business transactions are not execution guarantees.

## Conditions and validation

Skip tags apply to matching agents before each layer starts. Skipped agents
are excluded from fan-in; a fully skipped layer passes. Layer policies are
`any` (default), `all`, `quorum:N`, or `percent:P`. Choose `all` when every
independent result is required downstream.

Validation commands run after an agent reports pass and can fail that session.
Callbacks explicitly re-execute earlier work; they are distinct from skipping
work or retrying a failed layer. `pause_after` suspends at a configured layer
boundary until continued. See [workflow control](common-40-workflow.md).

## Trusted host execution

Authentication and project-scoped service tokens restrict application access.
Scripts and CLI agents execute with the host process's permissions and access
to configured credentials. Project scoping is not a hard sandbox for mutually
untrusted tenants. Limit access to workflow/script authors and use a deployment
boundary appropriate to the code being run.

Keep service tokens server-side, change the seeded admin password, and use
HTTPS for remote access. Run local HTTP with `--insecure-cookies` as described
in [installation](../README.md#installation). Back up the persistent data using
a consistent SQLite backup or a stopped server, and retain artifacts alongside
the database according to the configured local/object-storage backend.
