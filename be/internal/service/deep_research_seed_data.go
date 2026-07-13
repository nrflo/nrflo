package service

// Bundled definition of the global deep-research workflow. Seeded under
// GlobalProjectID at startup by EnsureGlobalDeepResearch. Edit here to change
// the shipped definition; existing seeded copies are not overwritten (the seed
// is create-if-absent), so bump by re-seeding into a fresh/admin-cleared slot.

// drFindingSchemas is the workflows.finding_schemas JSON for the workflow. The
// synthesize agent MUST emit the "report" key — web_deep_research reads it.
const drFindingSchemas = `[
{"key":"angles","schema":{"type":"object","required":["question","angles"],"properties":{"question":{"type":"string"},"angles":{"type":"array","minItems":1,"maxItems":6,"items":{"type":"object","required":["label","query"],"properties":{"label":{"type":"string"},"query":{"type":"string"},"rationale":{"type":"string"}}}}}},"example":{"question":"q","angles":[{"label":"broad","query":"q overview"},{"label":"recent","query":"q 2026"},{"label":"contrarian","query":"q criticism"}]}},
{"key":"claims","schema":{"type":"array","items":{"type":"object","required":["claim","quote","sourceUrl","sourceQuality","importance"],"properties":{"claim":{"type":"string"},"quote":{"type":"string"},"sourceUrl":{"type":"string"},"sourceQuality":{"enum":["primary","secondary","blog","forum","unreliable"]},"importance":{"enum":["central","supporting","tangential"]}}}},"example":[]},
{"key":"verdicts_a","schema":{"type":"array","items":{"type":"object","required":["claimRef","refuted","confidence"],"properties":{"claimRef":{"type":"string"},"refuted":{"type":"boolean"},"confidence":{"enum":["high","medium","low"]},"evidence":{"type":"string"},"counterSource":{"type":"string"}}}},"example":[]},
{"key":"verdicts_b","schema":{"type":"array","items":{"type":"object","required":["claimRef","refuted","confidence"],"properties":{"claimRef":{"type":"string"},"refuted":{"type":"boolean"},"confidence":{"enum":["high","medium","low"]},"evidence":{"type":"string"},"counterSource":{"type":"string"}}}},"example":[]},
{"key":"verdicts_c","schema":{"type":"array","items":{"type":"object","required":["claimRef","refuted","confidence"],"properties":{"claimRef":{"type":"string"},"refuted":{"type":"boolean"},"confidence":{"enum":["high","medium","low"]},"evidence":{"type":"string"},"counterSource":{"type":"string"}}}},"example":[]},
{"key":"report","schema":{"type":"object","required":["summary","findings","caveats"],"properties":{"summary":{"type":"string"},"findings":{"type":"array","items":{"type":"object","required":["claim","confidence","sources"],"properties":{"claim":{"type":"string"},"confidence":{"enum":["high","medium","low"]},"sources":{"type":"array","items":{"type":"string"}},"vote":{"type":"string"},"evidence":{"type":"string"}}}},"caveats":{"type":"string"},"openQuestions":{"type":"array","items":{"type":"string"}}}},"example":{"summary":"s","findings":[],"caveats":""}}
]`

const drScopePrompt = `You are the scope planner of a deep-research workflow. The user's research question is provided above as your instructions.

Caller-supplied context (may be empty — if blank, ignore it and research the question on its own terms):
${EXTERNAL_CONTEXT}

When that context is present, bias the angles toward what it implies the caller cares about (their domain, tech stack and versions, constraints, and what they already know) while still covering the question objectively.

Decompose it into 1-6 complementary web-search angles (a narrow or closed question may need only 1-2) that together cover the question from different directions (e.g. broad/primary, academic/technical, recent news, contrarian/skeptical, practitioner/implementation — adapt to the domain). Make each query specific enough to surface high-signal results; avoid redundancy.

Emit one finding with the emit_findings tool, key "angles", value {question, angles:[{label, query, rationale}]} (1-6 angles). If emit_findings returns an error, fix the value using the example in the error and call it again until it succeeds — do not call agent_finished while your finding is unsaved. After it succeeds, call agent_finished; if you cannot produce a valid value, call agent_fail with the reason.`

const drResearchPrompt = `You are the researcher of a deep-research workflow. The search angles from the previous layer are:
#{LAYER_FINDINGS:0}

For each angle:
1. Call web_search with the angle's query (you may batch multiple queries in one call).
2. From the results, pick diverse, credible sources; avoid loading many pages from the same domain.
3. Call web_fetch on the chosen URLs. If a page was offloaded to an artifact, use artifact_get / read_document to read the full content.
4. Extract 2-5 FALSIFIABLE claims per useful source. Each claim: a concrete checkable statement, a verbatim supporting quote, the sourceUrl, sourceQuality (primary|secondary|blog|forum|unreliable), and importance (central|supporting|tangential).

A failed/blocked fetch returns ok:false — note it and move on; do not invent content. Prioritize the most central, important claims and emit at most ~20 in total (drop tangential ones beyond that). Emit one finding with emit_findings, key "claims" (the array of all extracted claims). If emit_findings returns an error, fix the value using the example in the error and call it again until it succeeds — do not call agent_finished while your finding is unsaved. After it succeeds, call agent_finished; if you cannot produce a valid value, call agent_fail with the reason.`

const drVerifyAPrompt = `You are an adversarial claim verifier (lens: QUOTE SUPPORT). The claims to review are:
#{LAYER_FINDINGS:1}

Be skeptical. For each claim ask: is it actually entailed by its quote, or is it an overreach / misread / out-of-context? You MAY web_search for contradicting evidence. Default to refuted=true when uncertain.

To check the quote rigorously, read each claim's own source material: call artifact_list to see the researcher's cached pages (named websrc_*) and read the relevant ones with artifact_get, confirming the claim's quote appears verbatim and is not wrenched out of context. If a page is not cached you may web_search to locate it. Reference each claim by its 0-based index in the claims array above. Emit one finding with emit_findings, key "verdicts_a" = array of {claimRef (the claim's index as a string), refuted (bool), confidence (high|medium|low), evidence (specific), counterSource?}. If emit_findings returns an error, fix the value using the example in the error and call it again until it succeeds — do not call agent_finished while your finding is unsaved. After it succeeds, call agent_finished; if you cannot produce a valid value, call agent_fail with the reason.`

const drVerifyBPrompt = `You are an adversarial claim verifier (lens: INDEPENDENT CORROBORATION). The claims to review are:
#{LAYER_FINDINGS:1}

For each claim, web_search for independent sources that confirm or contradict it. refuted=true unless the claim is independently corroborated by a credible source other than the original. Default to refuted=true when uncertain.

Corroborate against INDEPENDENT sources only: web_search for each claim's key fact and prefer results from a different domain than the claim's sourceUrl. When a snippet is inconclusive, web_fetch at most ~5 of those new, different-domain pages to confirm or contradict — never fetch the claim's original source (that is not independent corroboration). refuted=true unless an independent source corroborates. Reference each claim by its 0-based index in the claims array above. Emit one finding with emit_findings, key "verdicts_b" = array of {claimRef (the claim's index as a string), refuted, confidence, evidence, counterSource?}. If emit_findings returns an error, fix the value using the example in the error and call it again until it succeeds — do not call agent_finished while your finding is unsaved. After it succeeds, call agent_finished; if you cannot produce a valid value, call agent_fail with the reason.`

const drVerifyCPrompt = `You are an adversarial claim verifier (lens: SOURCE QUALITY & RECENCY). The claims to review are:
#{LAYER_FINDINGS:1}

For each claim ask: is the source strong enough for the claim's strength (extraordinary claims need primary sources)? Is it stale for a fast-moving topic? Is it marketing / a press release / a cherry-picked benchmark? refuted=true for weak-source-for-strong-claim, outdated, or promotional. Default to refuted=true when uncertain.

Verify primarily from each claim's verbatim quote and sourceUrl (already provided above). Use web_search (it returns short snippets) sparingly — only to check for clearly contradicting or corroborating evidence. Do NOT open or read full pages, do NOT use any built-in web-browsing or fetch tool, and never repeat the same search — keep your context small so you do not exhaust it. Reference each claim by its 0-based index in the claims array above. Emit one finding with emit_findings, key "verdicts_c" = array of {claimRef (the claim's index as a string), refuted, confidence, evidence, counterSource?}. If emit_findings returns an error, fix the value using the example in the error and call it again until it succeeds — do not call agent_finished while your finding is unsaved. After it succeeds, call agent_finished; if you cannot produce a valid value, call agent_fail with the reason.`

const drSynthesizePrompt = `You are the synthesizer of a deep-research workflow.

Claims:
#{LAYER_FINDINGS:1}

Verdicts from the (up to three) independent verifiers:
#{LAYER_FINDINGS:2}

Each verdict's claimRef is the 0-based index of the claim in the claims array above — use it to map verdicts back to their claim.

A claim SURVIVES when a majority of the AVAILABLE verdicts for it are not-refuted (e.g. >=2 of 3, or >=2 of 2 when one verifier is absent). Drop refuted claims. Merge semantically duplicate survivors and combine their sources. Weight each finding's confidence by corroboration count and source quality (high = multiple primary/independent sources; low = single blog-quality source).

Emit one finding with emit_findings, key "report", value {summary (3-5 sentences directly answering the question), findings:[{claim, confidence (high|medium|low), sources:[url], vote (e.g. "2-1"), evidence}], caveats, openQuestions:[...]}. If emit_findings returns an error, fix the value using the example in the error and call it again until it succeeds — do not call agent_finished while your finding is unsaved. After it succeeds, call agent_finished; if you cannot produce a valid value, call agent_fail with the reason.`

// drAgent describes one seeded agent definition.
type drAgent struct {
	ID    string
	Layer int
	Model string
	Tools string
}

// drAgents is the layer-ordered roster. All agents run cli_interactive: the
// claude/codex CLIs self-authenticate, so the workflow needs no server-side API
// credential. The L2 verifiers are differentiated by lens so each reads the
// evidence its lens actually needs:
//
//	verify_a (QUOTE SUPPORT)            opus_4_8_1m + artifact_get — reads the
//	    researcher's cached source pages to check each quote verbatim/in-context
//	    (the 1M window holds them without exhausting).
//	verify_b (INDEPENDENT CORROBORATION) codex GPT-5.6 Sol (cross-provider diversity)
//	    + bounded web_fetch of NEW sources only — never the cached originals.
//	verify_c (SOURCE QUALITY/RECENCY)   lean sonnet, snippet-level web_search.
//
// quorum:2 tolerates one verifier failing (e.g. codex not configured).
var drAgents = []drAgent{
	{ID: "scope", Layer: 0, Model: "sonnet", Tools: "emit_findings"},
	{ID: "research", Layer: 1, Model: "sonnet", Tools: "web_search,web_fetch,read_document,artifact_get,emit_findings"},
	{ID: "verify_a", Layer: 2, Model: "opus_4_8_1m", Tools: "web_search,artifact_list,artifact_get,emit_findings"},
	{ID: "verify_b", Layer: 2, Model: "codex_gpt56_sol_high", Tools: "web_search,web_fetch,emit_findings"},
	{ID: "verify_c", Layer: 2, Model: "sonnet", Tools: "web_search,emit_findings"},
	{ID: "synthesize", Layer: 3, Model: "opus_4_8", Tools: "emit_findings"},
}

// drPrompt returns the prompt for a seeded agent id.
func drPrompt(id string) string {
	switch id {
	case "scope":
		return drScopePrompt
	case "research":
		return drResearchPrompt
	case "verify_a":
		return drVerifyAPrompt
	case "verify_b":
		return drVerifyBPrompt
	case "verify_c":
		return drVerifyCPrompt
	case "synthesize":
		return drSynthesizePrompt
	}
	return ""
}
