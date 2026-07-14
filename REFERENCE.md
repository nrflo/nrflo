# nrflo Reference

On-demand companion to [CLAUDE.md](CLAUDE.md) (the auto-loaded map). Read the relevant section here before editing docs or when routing to a feature's documentation.

Contents: [Doc Authoring](#doc-authoring-full-rule-1) · [Feature Index](#feature-index) · [Docker image](#docker-image)

## Doc Authoring (full Rule 1)

**Hard caps (bytes; enforced by reviewer and `.claude/skills/finalize/check_caps.sh`):**

| File | Cap |
|------|-----|
| Root CLAUDE.md | 10240 |
| be/CLAUDE.md, ui/CLAUDE.md | 12288 |
| Package CLAUDE.md (spawner exception: 15360) | 12288 |
| Sub-package / leaf CLAUDE.md | 6144 |

**Banned content:**
- ASCII-art box diagrams (┌─┐, ├──, pipes-and-dashes). Bullet lists or short tables instead.
- Copied Go interface or struct signatures — point to the .go file with `path:line` instead.
- Verbatim JSON/TOML/protocol payload samples longer than 10 lines — link to a test fixture or source.
- Per-test inventories (## Testing sections listing every test file). Use `make test-pkg PKG=<name>` as the universal pointer.
- Per-handler / per-endpoint / per-component enumerations that already exist in the file tree. List directory + grep hint.
- Status matrices duplicated across files — keep one copy, link from others.

**Populating (one pass, no thrash):**
1. Pick ONE owning package CLAUDE.md per changed area — deepest wins; no behavior change → no doc edit.
2. Add/update ≤3 sentences in the relevant section; anything longer goes to that package's `REFERENCE.md` (create if absent) with a one-line pointer left in CLAUDE.md.
3. Never reword existing unrelated content to free space.
4. Measure `wc -c` once at the END; if over cap, move the largest mechanics paragraph to REFERENCE.md — do not compress wording.

**Pointer hygiene** (from Anthropic skill-authoring guidance):
- Write pointers as plain paths or markdown links — NEVER a bare `@path`: that is Claude Code import syntax and loads the file at launch, defeating the cap.
- Keep pointers one level deep (CLAUDE.md → REFERENCE.md); don't chain REFERENCE → REFERENCE.
- A REFERENCE.md over ~100 lines gets a `Contents:` line at top (agents may partially read long files).
- Every pointer says when to read the target ("read before changing X"), since nothing auto-loads it.

## Feature Index

### Workflow execution
- **Layer execution, aggregation, callbacks** → [orchestrator](be/internal/orchestrator/CLAUDE.md)
- **Manual restart, retry-failed, orchestration entry points** → [orchestrator](be/internal/orchestrator/CLAUDE.md) + [api](be/internal/api/CLAUDE.md)
- **Low-context relaunch** → [spawner](be/internal/spawner/CLAUDE.md)
- **Stall detection / stall timeouts / restart cap** → [spawner](be/internal/spawner/CLAUDE.md)
- **Take-control / resume-session / exit-interactive / PTY relay** → [orchestrator](be/internal/orchestrator/CLAUDE.md) + [api](be/internal/api/CLAUDE.md)
- **Interactive start & plan mode** → [orchestrator](be/internal/orchestrator/CLAUDE.md)
- **Endless loop mode** → [orchestrator](be/internal/orchestrator/CLAUDE.md)
- **Merge conflict auto-resolution / push-after-merge** → [orchestrator](be/internal/orchestrator/CLAUDE.md)
- **Plan lifecycle & sub/dynamic workflows** (planner, revise/approve, self-drafting boundary, `run_subworkflow`/`dynamic_workflow`) → [orchestrator](be/internal/orchestrator/CLAUDE.md) + [service](be/internal/service/CLAUDE.md)

### Agents, templates, and configuration
- **Workflow / agent / system-agent definitions** → [spawner](be/internal/spawner/CLAUDE.md) + [service](be/internal/service/CLAUDE.md) + [doc/](doc/)
- **Default templates** → [service](be/internal/service/CLAUDE.md) + [api](be/internal/api/CLAUDE.md)
- **CLI models registry / supported models** → [spawner](be/internal/spawner/CLAUDE.md)

### Execution backends (`execution_mode`)
- **`api` — in-process Anthropic runner** → [spawner/apirun](be/internal/spawner/apirun/CLAUDE.md)
- **`cli_interactive` backend** → [spawner](be/internal/spawner/CLAUDE.md)
- **`script` — Python scriptBackend** → [spawner](be/internal/spawner/CLAUDE.md)
- **Per-project venv** → [venv/](be/internal/venv/)
- **Python tools (api-mode only)** → [spawner/apirun](be/internal/spawner/apirun/CLAUDE.md)
- **Python SDK + `script.context` socket method** → [sdk/python](be/internal/sdk/python/CLAUDE.md) + [socket](be/internal/socket/CLAUDE.md)
- **Provider capability matrix** → [capabilities.md](capabilities.md)

### Project-scoped & scheduled work
- **Project-scoped workflows** → [service](be/internal/service/CLAUDE.md) + [api](be/internal/api/CLAUDE.md)
- **Scheduled tasks** → [scheduler](be/internal/scheduler/CLAUDE.md)
- **Workflow chains and chain runs** → [be](be/CLAUDE.md) + [api](be/internal/api/CLAUDE.md)
- **Run trace timeline** (`GET /workflow-instances/{iid}/trace` + Trace UI tab) → [service](be/internal/service/CLAUDE.md)

### Auth & administration
- **Auth + sessions + login rate limit** → [auth](be/internal/auth/CLAUDE.md) + [api](be/internal/api/CLAUDE.md)
- **Route list, audit-log + user CRUD** → [api](be/internal/api/CLAUDE.md)
- **Service tokens** (long-lived project or global bearer tokens) → [api](be/internal/api/CLAUDE.md)

### Storage & operations
- **Artifact storage + agent runtime** (`NRF_ARTIFACTS_DIR`, `#{ARTIFACTS}`, `artifact_*` MCP tools) → [artifact/](be/internal/artifact/) + [service/artifact.go](be/internal/service/artifact.go)
- **Agent session logs + live sessions** → [api](be/internal/api/CLAUDE.md)
- **Per-project env vars** → [service](be/internal/service/CLAUDE.md)
- **DB schema, migrations, connection pool** → [db](be/internal/db/CLAUDE.md)
- **Observer agents (experimental)** → [spawner](be/internal/spawner/CLAUDE.md) + [api](be/internal/api/CLAUDE.md)

## Docker image

`ghcr.io/nrflo/nrflo-server` ([Dockerfile](Dockerfile)). Api-mode off by default; bundles Claude Code + codex CLIs (native musl, sha256-pinned) and poppler-utils (codex PDF extraction). Non-root; `/data`=`NRFLO_HOME` vol; logs `$NRFLO_HOME/logs/be.log`.
