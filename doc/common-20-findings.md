# Findings Patterns

Findings patterns pull data from other agents or project-level findings into an
agent's prompt. They are expanded after variable substitution.

---

## Validated Findings (`emit_findings`)

A workflow definition can register **finding schemas** under Settings: each is a
`key` plus a JSON Schema (Draft 2020) the value must satisfy, plus an example of
a valid value.

The `emit_findings` tool (all modes) / `c.findings.emit(key, value)` (Python
SDK) validates a value against the schema registered for `key`, then stores it
as a session finding. If the value does not match — or the key has no schema —
the call is **rejected, nothing is stored**, and the error returns the
validation message plus the example so you can correct the value and call again.

Use it when a downstream agent or hook expects a specific shape (e.g. an array
of `{file, severity}` objects). For free-form findings with no schema, keep
using `findings_add`.

**Reserved keys:** `_workflow_plan` is server-owned — its schema is resolved
ahead of (and can never be overridden by) a workflow's configured schemas.
Write it only with `emit_findings`; `findings_add`/`findings_append` (and
their bulk variants) reject it outright, naming `emit_findings` as the
correct tool.

---

## Agent Findings (`#{FINDINGS:...}`)

Pull prior agent findings into prompts. This is **template-keyed**: for a
fan-out agent (multiple nodes sharing one template), it aggregates findings
across every node running that template. Use `#{NODE_FINDINGS:...}` below to
target a single node.

**Syntax:**

```markdown
#{FINDINGS:setup-analyzer}
#{FINDINGS:setup-analyzer:summary}
#{FINDINGS:setup-analyzer:summary,files_to_modify}
```

**Output format (single agent):**

```
summary: Analysis found 3 files to modify
files_to_modify:
  - src/handler.go
  - src/service.go
```

**Output format (parallel agents — multiple models):**

```
- setup-analyzer:claude:opus:
  summary: Analysis found 3 files to modify
- setup-analyzer:claude:sonnet:
  summary: Found pattern in auth module
```

When requesting a single key from parallel agents:

```
- setup-analyzer:claude:opus: Analysis found 3 files
- setup-analyzer:claude:sonnet: Found pattern in auth module
```

**Missing findings placeholder:**

```
_No findings yet available from setup-analyzer_
```

---

## Project Findings (`#{PROJECT_FINDINGS:...}`)

Pull project-level findings into prompts.

**Syntax:**

```markdown
#{PROJECT_FINDINGS:architecture}
#{PROJECT_FINDINGS:architecture,conventions}
```

**Single key output:** Returns the value directly (no key prefix).

**Multiple keys output:**

```
architecture: Monorepo with Go backend and React frontend
conventions: Use camelCase for JS, snake_case for Go
```

**Missing key placeholder:**

```
_No project finding for key 'architecture'_
```

For multiple keys, each missing key gets its own placeholder while found keys
display normally.

---

## Layer Findings (`#{LAYER_FINDINGS:N}` / `#{PRIOR_LAYER_FINDINGS}`)

Pull a flat sibling roster for an entire layer into prompts — useful for merger
or aggregator agents that summarise what earlier-layer siblings produced.

**Syntax:**

```markdown
#{LAYER_FINDINGS:1}
#{PRIOR_LAYER_FINDINGS}
```

`#{PRIOR_LAYER_FINDINGS}` is shorthand for the layer immediately before the
current agent's layer. It renders `_No prior layer_` when the current agent is
on layer 0.

The roster is keyed by **node id**, not agent type: a static workflow has one
node per agent def (so the roster looks agent_type-keyed), but a fan-out layer
lists each sibling node separately under its own header.

**Output format** (nodes sorted alphabetically, findings two-space-indented):

```
analyzer:
  recommendation: Use database-level constraints
  risk_level: low
intel-merger:
  _No findings_
researcher:
  sources:
    - RFC 9110
    - internal design doc
```

**Example use — layer-2 merger reading layer-1 siblings:**

```markdown
## Layer 1 Results
#{PRIOR_LAYER_FINDINGS}

Synthesise the above into a final recommendation.
```

Nodes that have no session row for the workflow instance render as
`  _No findings_` under their node header.

---

## Node Findings (`#{NODE_FINDINGS:<node_id>}`)

Pull findings attributed to a single execution node — the specific slot in
the run, not the template it was spawned from. Use this over `#{FINDINGS:...}`
when a template fans out into multiple nodes and you need one sibling's
output rather than the aggregate.

**Syntax:**

```markdown
#{NODE_FINDINGS:implementor}
#{NODE_FINDINGS:implementor:summary}
#{NODE_FINDINGS:implementor:summary,files_to_modify}
```

**Output format** — same `key: value` shape as a single-agent findings block:

```
summary: Analysis found 3 files to modify
files_to_modify:
  - src/handler.go
  - src/service.go
```

**Unknown node id:** expands to an empty string and logs a server-side
warning (same convention as `#{ARTIFACT:name}`).

**Known node, no findings yet:**

```
_No findings yet available from implementor_
```

---

## Artifact Variables (`#{ARTIFACTS}` / `#{ARTIFACT:name}`)

Reference pre-materialized input artifacts directly in prompts.

**Syntax:**

```markdown
#{ARTIFACTS}
#{ARTIFACT:mydata.csv}
```

`#{ARTIFACTS}` expands to tab-separated `name\t/abs/path` lines for all
artifacts attached to the workflow instance, or
`_No artifacts available for this workflow._` when none exist.
`#{ARTIFACT:name}` expands to the absolute local path of the named artifact,
or an empty string (with a server-side warning) when not found.

Both vars materialize artifacts to `$NRF_ARTIFACTS_DIR` on first use;
subsequent calls are idempotent (size-match check).

**Example:**

```markdown
The following data files are available:
#{ARTIFACTS}

Analyze the primary dataset at: #{ARTIFACT:dataset.csv}
```
