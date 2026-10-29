# Use NRFLO from an application

Your application owns users, domain state, and business actions. NRFLO is a
separate execution service that runs defined stages and stores execution data.

## Prepare access

Install the server, create a project with a real working directory, and create
or import a **project-scoped** workflow. In **Settings → Administration → Service
Tokens**, mint a token scoped to that project. Use the same project ID in the
URL and `X-Project` header. Keep tokens on the application backend.

The commands below are real routes in this checkout. Set `NRFLO_SERVER_URL`,
`NRFLO_PROJECT`, and `NRFLO_TOKEN` to your deployment, project, and service token.
Import the [document-intake example](mixed-workflow.md) before starting it.

## Start and inspect a run

```bash
curl --fail-with-body -sS \
  -H "Authorization: Bearer $NRFLO_TOKEN" \
  -H "X-Project: $NRFLO_PROJECT" \
  -H 'Content-Type: application/json' \
  -d '{"workflow":"document-intake","external_id":"intake-001"}' \
  "$NRFLO_SERVER_URL/api/v1/projects/$NRFLO_PROJECT/workflow/run"
```

Save the response's `instance_id` as `NRFLO_INSTANCE`. This identifies one run,
including when the same workflow definition has multiple concurrent instances.
`external_id` is caller correlation metadata, not an idempotency key.

```bash
curl --fail-with-body -sS \
  -H "Authorization: Bearer $NRFLO_TOKEN" \
  -H "X-Project: $NRFLO_PROJECT" \
  "$NRFLO_SERVER_URL/api/v1/projects/$NRFLO_PROJECT/workflow?instance_id=$NRFLO_INSTANCE"
```

Use `all_workflows[instance_id]` to select the exact run. The response's `state`
can fall back to another run when the requested ID is absent; do not infer
successful lookup from HTTP 200 alone. Each run includes `status`, combined
`findings` (keys include node/model identifiers), and `workflow_final_result` when written.
The summary is optional; consume structured findings for machine decisions.

`waiting` and plan-related states require operator/plan action. Treat
`completed`, `project_completed`, and `failed` as terminal. Bound client timeouts and
backoff when reading status. The NRFLO web UI uses WebSockets for live updates.

For an existing instance, these read routes provide additional execution data:

- `GET /api/v1/workflow-instances/{instance_id}/artifacts`
- `GET /api/v1/workflow-instances/{instance_id}/trace`

## Inputs, errors, and lifecycle

The start request supports `instructions`, `seed_findings` (string values),
`external_id`, `external_context` (a string), and `input_artifacts`. Artifact
references identify previously staged uploads, not arbitrary host file paths.
[TransferDue](transferdue.md) demonstrates a ticket-scoped artifact integration;
its application-specific notification scripts deliver completion to its backend.

HTTP errors may be structured JSON or plain text. Save the returned
`X-Request-ID` for diagnosis. Do not blindly retry an ambiguous start response:
a run may already have started. Correlate and reconcile before launching again.
Run failure is separate from HTTP request failure.

[Execution semantics](execution-semantics.md) explains restart behavior and
external side effects. For an MCP client, use the supported
[external MCP bridge](mcp-external.md) with a project-scoped token.
