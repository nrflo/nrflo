-- Differentiate the L2 verifiers by lens (see service/deep_research_seed_data.go).
-- 000151 made all three lean+identical; this gives each the model + tools + evidence
-- its lens needs:
--   verify_a (QUOTE SUPPORT)             opus_4_8_1m + artifact reads of the cached
--                                        originals (1M window holds them).
--   verify_b (INDEPENDENT CORROBORATION) codex + bounded web_fetch of NEW sources.
--   verify_c                             unchanged (lean sonnet).
-- Fresh installs get this from the seed; these scoped UPDATEs realign installs
-- seeded under 000151. Idempotent: the lean anchor is gone after the REPLACE, and
-- the model/tools assignments are fixed values.

UPDATE agent_definitions
   SET model = 'opus_4_8_1m',
       tools = 'web_search,artifact_list,artifact_get,emit_findings',
       prompt = REPLACE(prompt,
         'Verify primarily from each claim''s verbatim quote and sourceUrl (already provided above). Use web_search (it returns short snippets) sparingly — only to check for clearly contradicting or corroborating evidence. Do NOT open or read full pages, do NOT use any built-in web-browsing or fetch tool, and never repeat the same search — keep your context small so you do not exhaust it. Reference each claim by its 0-based index in the claims array above.',
         'To check the quote rigorously, read each claim''s own source material: call artifact_list to see the researcher''s cached pages (named websrc_*) and read the relevant ones with artifact_get, confirming the claim''s quote appears verbatim and is not wrenched out of context. If a page is not cached you may web_search to locate it. Reference each claim by its 0-based index in the claims array above.'),
       updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
 WHERE project_id = '__global__' AND workflow_id = 'deep-research' AND id = 'verify_a';

UPDATE agent_definitions
   SET tools = 'web_search,web_fetch,emit_findings',
       prompt = REPLACE(prompt,
         'Verify primarily from each claim''s verbatim quote and sourceUrl (already provided above). Use web_search (it returns short snippets) sparingly — only to check for clearly contradicting or corroborating evidence. Do NOT open or read full pages, do NOT use any built-in web-browsing or fetch tool, and never repeat the same search — keep your context small so you do not exhaust it. Reference each claim by its 0-based index in the claims array above.',
         'Corroborate against INDEPENDENT sources only: web_search for each claim''s key fact and prefer results from a different domain than the claim''s sourceUrl. When a snippet is inconclusive, web_fetch at most ~5 of those new, different-domain pages to confirm or contradict — never fetch the claim''s original source (that is not independent corroboration). refuted=true unless an independent source corroborates. Reference each claim by its 0-based index in the claims array above.'),
       updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
 WHERE project_id = '__global__' AND workflow_id = 'deep-research' AND id = 'verify_b';
