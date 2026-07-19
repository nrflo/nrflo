---
name: model-audit
description: "Compare the unified models registry (per-mode model IDs, contexts, effort lists, and defaults) against what the providers actually ship today: codex app-server model/list, claude --help --effort, and the Anthropic/OpenAI docs. Reports delisted models, mode/effort drift, and duplicate mappings. Read-only — proposes a seeds migration, never mutates the DB. Invoked standalone or as the pre-release drift check."
---

# /model-audit

Detects drift between nrflo's model registry and provider reality. Read-only:
it never edits the DB or `capabilities.md`; the fix for confirmed drift is a
new `be/internal/db/migrations/` seeds migration. One `models` row owns both
mode-specific model IDs, contexts, and effort lists for a provider/model pair.

## Step 1 — Collect provider truth

**Codex (authoritative, machine-readable).** The app-server publishes the
per-model effort matrix:

```bash
python3 - <<'EOF'
import json, select, subprocess, time
p = subprocess.Popen(["codex","app-server"], stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, text=True)
def send(o): p.stdin.write(json.dumps(o)+"\n"); p.stdin.flush()
def receive(request_id, timeout=15):
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        ready, _, _ = select.select([p.stdout], [], [], deadline-time.monotonic())
        if not ready: break
        line = p.stdout.readline()
        if not line: break
        try: message = json.loads(line)
        except ValueError: continue
        if message.get("id") == request_id: return message
    raise TimeoutError(f"no app-server response for request {request_id}")
send({"jsonrpc":"2.0","id":1,"method":"initialize","params":{"clientInfo":{"name":"nrflo-model-audit","title":"audit","version":"0"}}})
receive(1)
send({"jsonrpc":"2.0","method":"initialized"})
send({"jsonrpc":"2.0","id":2,"method":"model/list","params":{}})
response = receive(2)
for it in response["result"]["data"]:
    efforts = [e["reasoningEffort"] for e in it.get("supportedReasoningEfforts") or []]
    print(f'{it["model"]}\t{",".join(efforts)}\tdefault={it.get("defaultReasoningEffort")}\thidden={it["hidden"]}')
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
sqlite3 "${NRFLO_HOME:-$HOME/.nrflo}/nrflo.data" "SELECT provider, id, cli_model, api_model, cli_context, api_context, cli_efforts, api_efforts, default_effort, enabled, price_in, price_out, price_cache_write, price_cache_read FROM models ORDER BY provider, id"
```

Also read the current seeds so the report distinguishes "user DB drift" from
"repo seeds drift": latest `*_model*` migrations under
`be/internal/db/migrations/`.

## Step 3 — Diff and report

One table per surface (codex CLI, claude CLI, anthropic API, openai API):

| finding | meaning |
|---|---|
| **delisted** | a non-empty `cli_model`/`api_model` maps to a model the corresponding provider surface no longer lists |
| **missing model** | provider ships a model with no row/mode mapping in the registry |
| **mode drift** | a mode is enabled where the provider does not ship that model, or missing where it does |
| **effort drift** | `cli_efforts` or `api_efforts` differs from that surface's supported list |
| **context drift** | `cli_context` or `api_context` differs from that surface's documented context window |
| **duplicate** | two enabled rows have the same non-empty `(provider, cli_model)` or `(provider, api_model)` mapping |
| **stale default** | `default_effort` no longer matches the provider default (informational) |
| **pricing drift** | `price_in`/`price_out`/`price_cache_write`/`price_cache_read` no longer match the provider's published per-MTok rates — these drive `model.PriceClass()` and `PlanModelTierClass`'s premium/mid/cheap tiering |

End with: (a) a verdict line `MODEL AUDIT: CLEAN` or `MODEL AUDIT: N findings`;
(b) for confirmed drift / missing models, a ready-to-paste draft of `models`
table `UPDATE`/`INSERT` statements — but do **not** create the migration file
unless the user asks.

## Hard rules

- Never mutate the DB, never write migrations unprompted, never touch
  `capabilities.md`.
- Missing binaries/credentials degrade to a partial audit with an explicit
  "not checked" list — never a failure.
- Quote sources for any web-derived claim.
