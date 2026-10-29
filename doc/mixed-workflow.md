# Run a mixed Python + agent workflow

This runnable starter example processes a synthetic supplier-intake text file.
Python prepares the input, an API agent extracts two fields, and Python checks
that each field quotes the source. It uses NRFLO's current v1.0 import format.
It is not a complete document-analysis application.

## Prerequisites

[Install NRFLO](../README.md#quick-start). Clone this repository onto the server
host and create a project whose root is the checkout. Python 3 must be on PATH.
The scripts read `examples/document-intake/fixture.txt` relative to that root.

For the mixed run, enable API mode in **Settings → Administration** and configure
an API-capable model and its provider credentials. Use its **NRFLO registry ID**,
not an assumed provider alias. Local providers are also supported when configured.

## Generate and import

From the checkout root, replace `YOUR_MODEL_ID` with the enabled registry ID:

```bash
python3 examples/document-intake/bundle.py --model YOUR_MODEL_ID > /tmp/document-intake.json
```

On **Workflows**, import the generated file into the project. The bundle embeds
its Python script sources and defines these real agent fields:

| ID | `layer` | `execution_mode` | Responsibility |
|----|---------|------------------|----------------|
| `prepare` | 0 | `script` | Read the fixture and write `document` |
| `extract` | 1 | `api` | Write `extraction` with `title` and `reference` |
| `validate` | 2 | `script` | Check types/source quotations; write `validated` and summary |

Each layer uses `all` fan-in. These stages are dependent and run sequentially.
The script IDs are remapped by the normal importer; no database edits are needed.

## Run and inspect

Start `document-intake` as a project workflow in the UI, or use the
[REST start request](application-integration.md). Open the run's findings and logs.

Expected `validate.validated` finding:

```json
{"title":"Supplier onboarding request","reference":"INTAKE-001"}
```

The final summary is `Validated document intake: INTAKE-001`.
If extraction is missing, malformed, empty, or unsupported by the source,
validation raises an error and the script step fails. The agent's response is
not guaranteed to pass. These checks establish shape and evidence membership,
not completeness or correctness of a real business decision.

## Reproducible check without a model

```bash
python3 examples/document-intake/bundle.py --fixture-agent > /tmp/document-intake-fixture.json
```

Import this variant into a separate project, or deliberately replace the example
workflow. It replaces only extraction with a clearly canned Python step. It
checks script lifecycle, handoff, and validation without provider calls; it does
not validate model reasoning. The fixture variant needs no API-mode enablement.

## Extend the process explicitly

For independent analyses, put agents in one layer and use `all` if the validator
needs every result. Use [skip tags](common-40-workflow.md#workflow-groups-skip-tags)
for configured conditional work. A later verifier can write correction
instructions and request a callback; this example fails validation directly
and does not automatically request rework. Configure pause-after-layer when a
human must review results before proceeding.

Use scripts for known rules, transforms, and external API calls; use agents for
interpretation. Keep side-effect reconciliation in the application as described
in [execution semantics](execution-semantics.md).
