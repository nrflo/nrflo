---
name: model-bench
description: >-
  Market scan for tier-model substitution: fetch live OpenRouter pricing,
  platform popularity, and contamination-resistant coding benchmarks
  (SWE-rebench, Terminal-Bench), reprice a real recent nrflo workload from the
  local DB against each candidate, and produce a ranked candidates table per
  tier (t1 executor / t2 extractor) with fallback-chain recommendations.
  Read-only — never edits the models registry; the fix is Settings → Models +
  PUT /api/v1/tier-models. Use when the user asks "are there cheaper/better
  models for our tiers", "check openrouter alternatives", or "re-rank model
  candidates". Optional argument: a console session id to use as the reference
  workload (default: the most recent delegation-heavy kdre-style session).
---

# /model-bench

Compares the models currently behind the delegate tiers against the open
market on three axes — price, real-world popularity, quality — then reprices
an actual recorded nrflo workload so the dollar figures are grounded in our
token shapes, not synthetic estimates. Complements `/model-audit` (registry
drift vs providers); this skill looks outward at candidates we do NOT run yet.

## Step 1 — Live pricing (OpenRouter public API, no auth)

```bash
curl -s https://openrouter.ai/api/v1/models | python3 -c "
import json,sys
d=json.load(sys.stdin)['data']
for m in d:
    p=m.get('pricing',{})
    pi=float(p.get('prompt',0) or 0)*1e6; po=float(p.get('completion',0) or 0)*1e6
    pcr=float(p.get('input_cache_read',0) or 0)*1e6
    print(f\"{m['id']}|{pi:.3f}|{po:.2f}|{pcr:.3f}|{m.get('context_length',0)}\")"
```

Filter to candidate families per tier plus the current tier models as
baselines (query the `models` table for what tier chains resolve to today —
`SELECT * FROM tier_models ORDER BY tier, position`, falling back to the
seeded sonnet/haiku defaults). Skip `:free` variants (rate-limited, no SLA)
and note `:batch` variants only as a footnote. A model with cache-read price
0.000 usually means no cache discount — bill its cache reads at full input
rate in Step 4, which typically disqualifies it (cache reads are ~95% of our
token volume).

## Step 2 — Popularity (deployment-proven signal)

Real token volume is the best available proxy for "tool-calling works in
agentic harnesses at scale". Sources, in order:

1. `WebFetch https://tokenmaxxing.com/openrouter-rankings` — top models by
   daily/30-day tokens (openrouter.ai/rankings itself is JS-rendered and
   returns no data to fetchers; don't burn time on it).
2. `WebSearch "openrouter rankings top models programming token share <month year>"`
   for the programming-category split and cross-checks.

## Step 3 — Quality (contamination-resistant first)

Vendor-reported SWE-bench Verified numbers are contamination-suspect (models
train on the issues). Rank by fresh-issue evals first:

1. `WebFetch https://swe-rebench.com/` — resolved-rate on GitHub issues
   posted AFTER training cutoffs, with cost-per-task. This is the primary
   quality axis for the t1 executor tier.
2. `WebSearch "<model> terminal-bench SWE-bench Pro benchmark <year>"` for
   Terminal-Bench (agentic terminal work — closest to our executor shape)
   and SWE-bench Pro. Label vendor self-reported numbers as such.
3. For the t2 extractor tier no coding benchmark maps cleanly; weight
   popularity (Step 2) and price. The adversarial-verify template pass is
   the quality net for extraction; say so in the caveats.

Discard candidates whose fresh-issue score collapses far below their vendor
claim (classic contamination signature) unless price alone justifies a
fallback-chained trial.

## Step 4 — Reprice a real workload from the local DB

Pull per-tier token totals from a recent delegation-heavy session (argument,
or newest `kind='console_chat'` with executor+extractor delegations):

```bash
sqlite3 ~/.nrflo/nrflo.data "
SELECT s.agent_type, SUM(COALESCE(json_extract(s.tokens_json,'\$.input_tokens'),0)),
  SUM(COALESCE(json_extract(s.tokens_json,'\$.cache_read_tokens'),0)),
  SUM(COALESCE(json_extract(s.tokens_json,'\$.cache_write_tokens'),0)),
  SUM(COALESCE(json_extract(s.tokens_json,'\$.output_tokens'),0)), round(SUM(s.cost_estimate),2)
FROM agent_sessions s
WHERE s.workflow_instance_id IN (SELECT id FROM workflow_instances WHERE origin_session_id='<SID>')
GROUP BY s.agent_type;"
```

Per candidate: `cost = in×price_in + cache_read×price_cache_read +
cache_write×price_in + out×price_out` (all per MTok; billing cache writes at
input rate is deliberately conservative — most OpenRouter providers cache
implicitly with no write surcharge).

## Step 5 — Candidates table + recommendation

Output (chat only — no repo files; scratch goes to /tmp):

- One table per tier: candidate, $/MTok (in/out/cache-rd), repriced workload
  cost, multiplier vs current, quality signal (fresh-issue score > vendor
  claim > popularity), context window.
- A recommended fallback chain per tier (`candidate → current-model`), so a
  bad run degrades to today's behavior instead of failing.
- Mandatory caveats: candidates run api-mode via the OpenRouter provider
  (apirun toolset, no CLI); repriced token volumes are shaped by the current
  model's agentic behavior; extractor swaps are low-risk (read-only +
  verify pass), executor swaps ride on the caller's merge review.
- End with the sources used.

Do not edit the DB, the models registry, or tier chains — offer to wire them
(Settings → Models rows + `PUT /api/v1/tier-models/{tier}`) as a follow-up
only.
