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
baselines. When a vendor ships a flagship as a price-tiered family (e.g.
GPT-5.6 Sol/Terra/Luna), enumerate EVERY sibling — the cheap siblings are
exactly the tier candidates, and family names don't match the flagship's
grep pattern (query the `models` table for what tier chains resolve to today —
`SELECT * FROM tier_models ORDER BY tier, position`, falling back to the
seeded sonnet/haiku defaults). Skip `:free` variants (rate-limited, no SLA)
and note `:batch` variants only as a footnote. A model with cache-read price
0.000 usually means no cache discount — bill its cache reads at full input
rate in Step 4, which typically disqualifies it (cache reads are ~95% of our
token volume).

## Step 2 — Popularity (deployment-proven signal)

Real token volume is the best available proxy for "tool-calling works in
agentic harnesses at scale". The candidate pool is the FULL top-20 of the
OpenRouter leaderboard — every entry gets priced and considered, not just
pre-picked families; note the week-over-week growth % too (a +400% riser is
the market discovering a price/quality outlier before benchmarks catch up).
Sources, in order:

1. Ask the user to paste openrouter.ai/rankings if they have it open — the
   page is JS-rendered and returns no data to fetchers, and mirrors truncate
   it (tokenmaxxing showed 12 of 20 entries when checked).
2. `WebFetch https://tokenmaxxing.com/openrouter-rankings` — partial mirror,
   better than nothing.
3. `WebSearch "openrouter rankings top models programming token share <month year>"`
   for the programming-category split and cross-checks.

Drop from the pool only for concrete reasons, stated in the output: `:free`
variants (no SLA), no tool-calling support, context window too small for the
tier, or no cache-read discount (see Step 1).

## Step 3 — Quality (contamination-resistant first, per-tier axes)

Vendor-reported SWE-bench Verified numbers are contamination-suspect (models
train on the issues). Weight benchmarks by what each tier actually does:

**t1 executor (edits code agentically in a worktree):**
1. `WebFetch https://swe-rebench.com/` — resolved-rate on GitHub issues
   posted AFTER training cutoffs, with cost-per-task. Primary axis.
2. Terminal-Bench and SWE-bench Pro (multi-language, standardized scaffold —
   the contamination-resistant SWE-bench successor) via WebSearch.
3. Artificial Analysis Coding Agent Index (independent aggregator, standard
   suites — label as gameable) and Aider polyglot for edit-format
   discipline. LiveCodeBench/Codeforces measure competitive programming, NOT
   agentic repo work — a model can top them and still flop on fresh issues
   (observed: DeepSeek V4 Pro 93.5% LiveCodeBench vs 40.2% SWE-rebench).

**t2 extractor (read-and-report, one question one answer):**
SWE-bench barely applies. Weight instead: tau²-bench / tool-calling evals
(does it drive tools correctly), IFBench-style instruction following (does
it answer exactly what was asked), long-context retrieval (MRCR/RULER — the
brief points it at big files), then popularity and price. The
adversarial-verify template pass is the quality net; say so in the caveats.

Discard candidates whose fresh-issue score collapses far below their vendor
claim (classic contamination signature) unless price alone justifies a
fallback-chained trial. A model with NO independent benchmark data ranks on
popularity + price alone and must be labeled "unbenchmarked wildcard".

## Step 3.5 — Speed and reliability

Speed is tier-critical, not cosmetic: extractor findings return inline only
within the delegate call's wait window (default 120s), so t2 TTFT + output
tok/s decide inline-vs-spilled; slow t1 generation burns the executor
timeout budget on the same work.

1. Per-provider operational data (populated unauthenticated: uptime; the
   latency/throughput fields usually come back null without auth — take
   them when present, skip when null):

```bash
curl -s "https://openrouter.ai/api/v1/models/<author>/<slug>/endpoints" | python3 -c "
import json,sys
for e in json.load(sys.stdin)['data']['endpoints']:
    print(e['provider_name'], e.get('uptime_last_1d'), e.get('throughput_last_30m'), e.get('latency_last_30m'))"
```

   Flag candidates whose serving pool is dominated by <95%-uptime providers
   — OpenRouter routes across them and a flaky pool means retries eat the
   inline window.
2. Median output tok/s + TTFT: `WebSearch "<model> output tokens per second
   artificial analysis"` (Artificial Analysis publishes both). Vendor
   claims about "fewer output tokens per task" also count — a model that
   solves in half the tokens is effectively 2x faster at equal tok/s.

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
  claim > popularity), speed (tok/s + TTFT, with a worst-provider-uptime
  flag), context window.
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
