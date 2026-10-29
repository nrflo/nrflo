# Positioning verification

Checked against this checkout and the maintainer-provided brief on 2026-10-29.
NRFLO is presented as a self-hosted execution service for structured AI
workflows. Runtime interfaces and schemas are unchanged by this refresh.

## Claim ledger

Paths below are source evidence, not public feature inventories. Confidence
means implementation evidence was inspected; it does not imply every provider
combination was re-tested.

| Public claim | Source / test evidence | Confidence | Surface |
|--------------|------------------------|------------|---------|
| Python scripts share lifecycle and findings with agents | `be/internal/spawner/backend_script.go`, `be/internal/sdk/python/nrflo_sdk.py`; isolated tutorial smoke | Verified | README, homepage, tutorial |
| Ordered layers and optional parallel execution | `be/internal/orchestrator/orchestrator_loop.go`, `orchestrator_spawn.go`; capability s10 | Verified | README, homepage |
| Configurable fan-in excludes skips; all-skipped layers pass | `be/internal/service/layer_policy.go`, `be/internal/orchestrator/orchestrator_skip.go` | Verified | Concepts, semantics, capabilities |
| Validation and callback rework are configured controls | `be/internal/spawner/validation.go`, `be/internal/orchestrator/orchestrator_callback_plan.go`, package references | Verified | README, homepage, semantics |
| Layer-boundary human checkpoints | `be/internal/orchestrator/pause.go`, `continue.go` | Verified | Homepage, semantics |
| External application starts/inspects project runs | `be/internal/api/handlers_project_workflow.go`, `be/internal/types/request.go`; isolated token/import/start/read smoke | Verified | Application guide, homepage |
| Service tokens and external MCP | `be/internal/api/handlers_service_tokens.go`, `be/internal/api/REFERENCE.md`, `doc/mcp-external.md` | Verified | README, homepage, integration guide |
| API mode is NRFLO's in-process runtime | `be/internal/spawner/apirun/runner.go`, provider implementations, `be/go.mod` | Verified | Runtime table and website |
| CLI coding workflows, worktrees, merge, supported takeover | `be/internal/spawner/CLAUDE.md`, `be/internal/orchestrator/REFERENCE.md`; capability matrix has some skipped interaction scenarios | Implemented; runtime coverage qualified | Coding section and guide |
| Local OpenAI-compatible providers | `be/internal/service/custom_provider.go`, `doc/local-providers.md` | Implemented; configuration-dependent | Runtime links |
| Scheduled runs | `be/internal/scheduler/scheduler_dispatch.go` | Verified | Homepage mechanics |
| Startup fails orphaned in-flight work | `be/internal/api/server_shutdown.go`, `server_startup_sweep_test.go` | Verified | Execution semantics, FAQ |
| Artifact and live inspection facilities | `be/internal/api/server_routes_instance.go`, `be/internal/service/artifact.go`, `trace.go`; retained actual UI captures | Verified | Integration guide, screenshots |
| Single Go server, embedded UI, SQLite, Apache-2.0 | `be/cmd/server/main.go`, `be/internal/static/embed.go`, `be/internal/db/`, `Dockerfile`, `LICENSE` | Verified | README, metadata, website |

Some implementation symbols live in split files; package references describe
those flows. The generated [capability matrix](capabilities.md) retains its
original timestamp and skipped cases; it has not been rewritten as new proof.

## TransferDue verification

The user explicitly supplied `../dd-buyer` as the integration example. Inspected:

- `be/internal/report/nrflo.go` and `nrflo_test.go`: artifact upload, ticket creation,
  ticket workflow start, bearer/project headers, external context, instance ID.
- `be/internal/report/CLAUDE.md` and `REFERENCE.md`: service boundary and backend adapter.
- `scripts/nrflo/provision.py`: script and agent definitions, classification skips,
  validation/rule stages, report gate, and notification stage.
- `scripts/nrflo/notify.py`, `post_completed.py`, and `finalize_failed.py`:
  application-owned result assembly and callback/failure delivery.
- `be/internal/api/handlers_nrflo*.go` and tests: receiving application lifecycle.

The public case study describes architecture without private documents, secrets,
customer data, deployment addresses, throughput, uptime, or savings. It explicitly
discloses common authorship. Application-specific webhooks are distinguished
from a built-in generic delivery guarantee.

## Published surfaces and boundaries

- Product README, CLI help, and Homebrew release description use the workflow-engine identity; coding
  setup, installation commands, console, and MCP remain available.
- `doc/`: onboarding index, mixed workflow tutorial, precise application requests,
  execution boundaries, coding guide, and verified TransferDue case study.
- Embedded Common docs: workflow-first introduction links to the new guides.
  Python examples use the actual keyword-only findings signature and clarify
  that lifecycle calls do not exit the Python interpreter.
- Sibling website: hero, illustrative workflow, example, integration, use cases,
  coding section, runtimes, FAQ, mobile navigation, metadata, and social banner.
  Authentic screenshots remain identified as engineering-run captures.
- GitHub About and topics: applied through the repository API; homepage remains
  `https://nrflo.com/`. The generated social PNG is ready for manual GitHub
  social-preview upload in repository settings.

The website keeps its existing single-page routes and uses meaningful anchors
plus substantial linked guides. No empty marketing routes were added. Native
and Docker install commands were preserved and checked against the CLI,
Dockerfile, and release configuration; installers were not executed on the host.

## Validation and remaining limits

- Staged website production build/type check and desktop/mobile browser checks pass.
  Verified H1, anchor destinations, image loading, mobile API navigation,
  screenshot lightbox Escape behavior, horizontal overflow, and page errors.
- The fixture workflow imported and completed on an isolated server built from
  this checkout, using a project-scoped service token and real Python SDK calls.
  Its expected findings and final summary matched. Invalid source evidence
  failed validation and emitted no success summary.
- The script → API-agent → script bundle imported with an enabled model registry
  ID. A live provider call was not run; model extraction quality is unverified.
- `make build-ui`, `make filesize`, documentation caps, and local link checks pass.
- `make test` passes; `make test-ui` passes (448 files, 4,566 tests; 57.70s).
  Initial concurrent runs passed all assertions but exceeded the timing gates
  (backend 79s, frontend 87s); sequential rechecks passed those gates.

No HA, exactly-once actions, hardened tenant isolation, automatic crash resumption,
turnkey connectors, customer endorsement, or measured cost reduction is claimed.
Website pushes to the production branch deploy through existing Workers Builds.

## Repository Markdown alignment audit

The tracked Markdown inventory was checked for product identity, execution-mode
claims, onboarding paths, and relative file links. The root overview (also loaded
through the AGENTS.md symlink), provider references, console guidance, recovery
semantics, embedded-document publication guidance, and coding-example entry point
now agree with the workflow-engine positioning and current implementation.
Model IDs, contexts, and effort defaults resolve to the installed registry instead
of duplicated documentation tables. The backlog distinguishes supported finalizers
from its proposed failure-policy extension. Historical migration notes, generated
capability results, licenses, and coding test fixtures retain their original roles.
