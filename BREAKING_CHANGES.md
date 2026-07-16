# Breaking Changes and Migration Guides

This file documents releases that intentionally change NRFLO's external
contracts. Read the relevant entry before upgrading the server and update all
REST, WebSocket, workflow-authoring, and automation clients at the same time.

## Unreleased: unified model registry

This entry applies when upgrading from `0.7.8` or earlier to the release that
introduces the unified model registry. The bundled web UI is already updated.
Agent definitions, system-agent definitions, and historical session references
stored in NRFLO's database are migrated automatically. Observer settings,
external callers, and externally stored workflow payloads require the actions
below.

### Who must migrate

Migration is required for consumers that do any of the following:

- call `/api/v1/cli-models` or `/api/v1/api-models`;
- consume `cli_model.*` or `api_model.*` WebSocket events;
- create or update workflows, agents, observers, or console sessions using old
  model aliases;
- maintain custom CLI or API model rows;
- depend on an inherited model reasoning effort.

Consumers that do not use model administration or model identifiers are not
affected by this change.

### Before upgrading

1. While the old server is running, export custom model rows from both old
   endpoints.
2. Check for a custom CLI row and API row sharing the same ID. They may share a
   unified row only when they represent the same provider/model. If their
   providers differ, create a replacement under a new ID, update its users, and
   delete the colliding row before upgrading.
3. Likewise, move custom rows whose IDs collide with seeded built-in IDs.
   Built-in rows win such collisions during migration; current selectable IDs
   are in the [CLI](doc/cli.md#supported-models) and
   [API](doc/api.md#model-and-provider-selection) documentation.
   `gpt-5.5-mini` is also reserved as a disabled historical row.
4. Inventory external workflow JSON, scripts, and service integrations for the
   old paths, fields, events, and model IDs listed below.
5. Stop `nrflo_server` and back up the database from the configured
   `NRFLO_HOME` (default `~/.nrflo`):

   ```bash
   cp "${NRFLO_HOME:-$HOME/.nrflo}/nrflo.data" \
      "${NRFLO_HOME:-$HOME/.nrflo}/nrflo.data.pre-unified-models"
   ```

### REST endpoint migration

The two model collections are replaced by one global collection. The old
routes are not aliases; callers that keep using them receive `404`.

| Before | After |
|--------|-------|
| `GET|POST /api/v1/cli-models` | `GET|POST /api/v1/models` |
| `GET|PATCH|DELETE /api/v1/cli-models/{id}` | `GET|PATCH|DELETE /api/v1/models/{id}` |
| `POST /api/v1/cli-models/{id}/test` | `POST /api/v1/models/{id}/test` |
| `GET|POST /api/v1/api-models` | `GET|POST /api/v1/models` |
| `GET|PATCH|DELETE /api/v1/api-models/{id}` | `GET|PATCH|DELETE /api/v1/models/{id}` |

The test endpoint probes CLI mode only. It rejects a row whose `cli_model` is
empty. All writes remain admin-only; reads and the test endpoint require an
authenticated caller.

### JSON field migration

One model object now contains optional CLI and API modes:

| Old CLI field | Old API field | Unified field |
|---------------|---------------|---------------|
| `cli_type` | `provider` | `provider` (`claude` becomes `anthropic`; `codex` becomes `openai`) |
| `mapped_model` | `mapped_model` | `cli_model` / `api_model` |
| `supported_efforts` | `supported_efforts` | `cli_efforts` / `api_efforts` |
| `context_length` | `context_length` | `cli_context` / `api_context` |
| `reasoning_effort` | `reasoning_effort` | `default_effort` |
| `fallback_models` | — | `fallback_models` |

At least one of `cli_model` or `api_model` must be non-empty. A non-empty
`default_effort` must be accepted by every enabled mode on the row. Per-agent
`reasoning_effort` remains the way to override the row default.

Agent-definition writes that reference an unknown model, a model that does not
support the definition's execution mode, or an unsupported `reasoning_effort`
now fail with `400` (previously `500`). Clearing a mode's model string on a
custom row is rejected while definitions still use the row in that mode.

Example unified custom row:

```json
{
  "id": "custom-gpt", "provider": "openai", "display_name": "Custom GPT",
  "cli_model": "custom-gpt", "api_model": "custom-gpt",
  "cli_efforts": ["low", "high"], "api_efforts": ["low", "high"],
  "cli_context": 200000, "api_context": 200000, "default_effort": "low"
}
```

### WebSocket event migration

| Before | After |
|--------|-------|
| `cli_model.created`, `api_model.created` | `model.created` |
| `cli_model.updated`, `api_model.updated` | `model.updated` |
| `cli_model.deleted`, `api_model.deleted` | `model.deleted` |

The event payload continues to identify the affected row with `model_id`.

### Model ID migration

Agent definitions, system-agent definitions, and historical session references
are rewritten automatically. Global/project observer configuration and old
aliases in workflow-level `observer_model` fields are not comprehensively
rewritten; update those settings before launching observers. External payloads
must use the new ID and move any effort encoded in the old ID into the agent's
`reasoning_effort` field.

| Old ID or pattern | Final model ID | Effort to preserve |
|-------------------|----------------|--------------------|
| `sonnet` | `sonnet-5` | existing override/default |
| `haiku` | `haiku-4-5` | existing override/default |
| `opus_4_6`, `opus_4_7`, `opus_4_8` | `opus-4-6`, `opus-4-7`, `opus-4-8` | existing override/default |
| `opus_4_6_1m`, `opus_4_7_1m`, `opus_4_8_1m` | `opus-4-6-1m`, `opus-4-7-1m`, `opus-4-8-1m` | existing override/default |
| CLI `codex_gpt_normal`, `codex_gpt_high` | `gpt-5.2` | `high` |
| API `gpt53_codex_low|medium|high` | `gpt-5.3-codex` | suffix value |
| `codex_gpt54_normal`, `codex_gpt54_high` | `gpt-5.4` | `medium`, `high` |
| `gpt54_low|medium|high` | `gpt-5.4` | suffix value |
| `codex_gpt54_mini_low` | `gpt-5.4-mini` | `low` |
| `codex_gpt55_normal`, `codex_gpt55_high` | `gpt-5.5` | `medium`, `high` |
| `gpt55_low|medium|high` | `gpt-5.5` | suffix value |
| `codex_gpt55_mini_low` | `gpt-5.6-luna` | `low` |
| `codex_gpt56_sol_normal`, `codex_gpt56_sol_high` | `gpt-5.6-sol` | `medium`, `high` |
| `gpt56_sol_low|medium|high` | `gpt-5.6-sol` | suffix value |
| `codex_gpt56_terra_normal`, `codex_gpt56_terra_high` | `gpt-5.6-terra` | `medium`, `high` |
| `codex_gpt56_luna_low` | `gpt-5.6-luna` | `low` |

`gpt-5.3-codex` remains available for API mode but is no longer available for
CLI mode; use `gpt-5.2` for CLI callers. `gpt-5.5-mini` is disabled; use
`gpt-5.6-luna` with effort `low`.

### Changed inherited defaults

Callers that require stable behavior should send an explicit
`reasoning_effort` instead of inheriting the model-row default.

| Model | Previous default | New default |
|-------|------------------|-------------|
| `gpt-5.4-mini` | `low` | `medium` |
| `gpt-5.6-sol` | `medium` | `low` |
| `gpt-5.6-luna` | `low` | `medium` |

### After upgrading

1. Start the new server. Forward-only migrations run automatically.
2. Call `GET /api/v1/models` and confirm every required row is enabled and has
   a non-empty `cli_model` or `api_model` for the mode your caller uses.
3. Update model-administration clients to the unified response schema.
4. Update WebSocket subscriptions to `model.*`.
5. Replace old model IDs in external workflow definitions and automation, then
   re-save every global, project, and workflow observer model with a current ID.
6. For each required CLI row, run `POST /api/v1/models/{id}/test`.
7. Exercise at least one CLI-mode and one API-mode workflow before resuming
   schedules or unattended workflow launches.

### Rollback

Database migrations are forward-only and remove the old `cli_models` and
`api_models` tables. Do not run an older binary against the migrated database.
To roll back, stop the server, restore the pre-upgrade database backup, and then
restore the older binary.
