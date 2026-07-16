---
name: model-audit
description: "Compare the cli_models/api_models registry (models + supported_efforts) against what the providers actually ship today: codex app-server model/list (authoritative per-model effort matrix), claude --help --effort enum, and the Anthropic/OpenAI docs. Reports delisted models, effort drift, and duplicate rows. Read-only — proposes a seeds migration, never mutates the DB. Invoked standalone or as the pre-release drift check."
---

# /model-audit

Detects drift between nrflo's model registry and provider reality. Read-only:
it never edits the DB or `capabilities.md`; the fix for confirmed drift is a
new `be/internal/db/migrations/` seeds migration (supported_efforts lives on
the model rows — see migration `000166`).

## Step 1 — Collect provider truth

**Codex (authoritative, machine-readable).** The app-server publishes the
per-model effort matrix:

```bash
python3 - <<'EOF'
import json, subprocess, time
p = subprocess.Popen(["codex","app-server"], stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, text=True)
def send(o): p.stdin.write(json.dumps(o)+"\n"); p.stdin.flush()
send({"jsonrpc":"2.0","id":1,"method":"initialize","params":{"clientInfo":{"name":"nrflo-model-audit","title":"audit","version":"0"}}})
send({"jsonrpc":"2.0","method":"initialized"})
send({"jsonrpc":"2.0","id":2,"method":"model/list","params":{}})
deadline = time.time()+15
while time.time() < deadline:
    line = p.stdout.readline()
    if not line: break
    try: m = json.loads(line)
    except ValueError: continue
    if m.get("id") == 2:
        for it in m["result"]["data"]:
            efforts = [e["reasoningEffort"] for e in it.get("supportedReasoningEfforts") or []]
            print(f'{it["model"]}\t{",".join(efforts)}\tdefault={it.get("defaultReasoningEffort")}\thidden={it["hidden"]}')
        break
p.kill()
EOF
```

Skip (and say so) if `codex` is not on PATH.

**Claude CLI.** `claude --help | grep -A1 -- --effort` — the enum in the flag
help (currently `low, medium, high, xhigh, max`) is the CLI-side truth.

**API docs (web).** WebSearch/WebFetch, quoting what you find:
- Anthropic `output_config.effort`: which levels, which models (4.6+ vs
  budget-only haiku), whether new levels/models appeared.
- OpenAI Responses `reasoning.effort` for the gpt-5.x family (note: `ultra`
  is codex-only; do not expect it in the API).

## Step 2 — Read the registry

Run against the real DB (honor `NRFLO_HOME`):

```bash
sqlite3 ~/.nrflo/nrflo.data "SELECT cli_type, id, mapped_model, reasoning_effort, supported_efforts, enabled FROM cli_models ORDER BY cli_type, mapped_model"
sqlite3 ~/.nrflo/nrflo.data "SELECT provider, id, mapped_model, reasoning_effort, supported_efforts, enabled FROM api_models ORDER BY provider, mapped_model"
```

Also read the current seeds so the report distinguishes "user DB drift" from
"repo seeds drift": latest `*_model*` migrations under
`be/internal/db/migrations/`.

## Step 3 — Diff and report

One table per surface (codex CLI, claude CLI, anthropic API, openai API):

| finding | meaning |
|---|---|
| **delisted** | registry row maps to a model the provider no longer lists (e.g. codex dropped it from model/list) |
| **missing model** | provider ships a model with no registry row |
| **effort drift** | row's `supported_efforts` ≠ provider's list for that mapped_model |
| **duplicate** | two enabled rows with identical (mapped_model, reasoning_effort) |
| **stale default** | row's `reasoning_effort` no longer matches the provider default (informational) |

End with: (a) a verdict line `MODEL AUDIT: CLEAN` or `MODEL AUDIT: N findings`;
(b) for effort drift / missing models, a ready-to-paste draft of the seeds
migration `UPDATE`/`INSERT` statements — but do **not** create the file unless
the user asks.

## Hard rules

- Never mutate the DB, never write migrations unprompted, never touch
  `capabilities.md`.
- Missing binaries/credentials degrade to a partial audit with an explicit
  "not checked" list — never a failure.
- Quote sources for any web-derived claim.
