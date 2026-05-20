# Findings Patterns

Findings patterns pull data from other agents or project-level findings into an
agent's prompt. They are expanded after variable substitution.

---

## Agent Findings (`#{FINDINGS:...}`)

Pull prior agent findings into prompts.

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

**Output format** (agents sorted alphabetically, findings two-space-indented):

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

Agents that have no session row for the workflow instance render as
`  _No findings_` under their agent_type header.

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
