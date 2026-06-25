-- Revert the verifier split back to the lean uniform verifiers of 000151.
UPDATE agent_definitions
   SET model = 'sonnet',
       tools = 'web_search,emit_findings',
       prompt = REPLACE(prompt,
         'To check the quote rigorously, read each claim''s own source material: call artifact_list to see the researcher''s cached pages (named websrc_*) and read the relevant ones with artifact_get, confirming the claim''s quote appears verbatim and is not wrenched out of context. If a page is not cached you may web_search to locate it. Reference each claim by its 0-based index in the claims array above.',
         'Verify primarily from each claim''s verbatim quote and sourceUrl (already provided above). Use web_search (it returns short snippets) sparingly — only to check for clearly contradicting or corroborating evidence. Do NOT open or read full pages, do NOT use any built-in web-browsing or fetch tool, and never repeat the same search — keep your context small so you do not exhaust it. Reference each claim by its 0-based index in the claims array above.'),
       updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
 WHERE project_id = '__global__' AND workflow_id = 'deep-research' AND id = 'verify_a';

UPDATE agent_definitions
   SET tools = 'web_search,emit_findings',
       prompt = REPLACE(prompt,
         'Corroborate against INDEPENDENT sources only: web_search for each claim''s key fact and prefer results from a different domain than the claim''s sourceUrl. When a snippet is inconclusive, web_fetch at most ~5 of those new, different-domain pages to confirm or contradict — never fetch the claim''s original source (that is not independent corroboration). refuted=true unless an independent source corroborates. Reference each claim by its 0-based index in the claims array above.',
         'Verify primarily from each claim''s verbatim quote and sourceUrl (already provided above). Use web_search (it returns short snippets) sparingly — only to check for clearly contradicting or corroborating evidence. Do NOT open or read full pages, do NOT use any built-in web-browsing or fetch tool, and never repeat the same search — keep your context small so you do not exhaust it. Reference each claim by its 0-based index in the claims array above.'),
       updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
 WHERE project_id = '__global__' AND workflow_id = 'deep-research' AND id = 'verify_b';
