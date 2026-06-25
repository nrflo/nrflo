-- Revert the deep-research verifiers to a lean, context-bounded toolset. Migration
-- 000149 gave them web_fetch + artifact_list/artifact_get and told them to read the
-- researcher's cached full-page markdown (websrc_*) — in practice verify_a read all
-- ~18 cached pages into a sonnet context and exhausted it, triggering an endless
-- low-context relaunch loop that stalled the quorum:2 layer. Verifiers now work from
-- each claim's verbatim quote + sourceUrl (already in the claims they receive) plus
-- sparing web_search snippets. Fresh installs get this from the seed; these UPDATEs
-- realign installs that ran 000149 (0 rows otherwise). Idempotent: the REPLACE anchor
-- no longer exists after it runs.
UPDATE agent_definitions
   SET tools = 'web_search,emit_findings',
       updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
 WHERE project_id = '__global__' AND workflow_id = 'deep-research'
   AND id IN ('verify_a', 'verify_b', 'verify_c');

UPDATE agent_definitions
   SET prompt = REPLACE(prompt,
       'You have artifact_list/artifact_get to read pages the researcher already fetched (named websrc_*), plus web_search/web_fetch to gather more. Prefer reading an already-fetched page over re-fetching it; use only these nrflo tools (not any built-in web-browsing tool); and never fetch or search the same URL or query twice — record your verdict and move on. Reference each claim by its 0-based index in the claims array above.',
       'Verify primarily from each claim''s verbatim quote and sourceUrl (already provided above). Use web_search (it returns short snippets) sparingly — only to check for clearly contradicting or corroborating evidence. Do NOT open or read full pages, do NOT use any built-in web-browsing or fetch tool, and never repeat the same search — keep your context small so you do not exhaust it. Reference each claim by its 0-based index in the claims array above.')
 WHERE project_id = '__global__' AND workflow_id = 'deep-research'
   AND id IN ('verify_a', 'verify_b', 'verify_c');
