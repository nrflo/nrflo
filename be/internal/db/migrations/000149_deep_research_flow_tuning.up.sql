-- Tune the bundled global deep-research workflow from observed live-run behaviour.
-- Definition is seeded create-if-absent (service/deep_research_seed*.go), so these
-- scoped in-place UPDATEs bring already-seeded installs in line with the new seed
-- (fresh installs get it from the seed; 0 rows changed otherwise). UPDATE-in-place,
-- NOT delete-and-reseed: workflow_instances FK workflows ON DELETE CASCADE, so
-- dropping the def would destroy past runs + findings.

-- 1. Completion contract: emit the finding, retry on schema error, never finish with
--    an unsaved finding, else agent_fail. Replaces the bare "Then call agent_finished."
--    (added in 000148) across all 6 agents.
UPDATE agent_definitions
   SET prompt = REPLACE(prompt, 'Then call agent_finished.',
       'If emit_findings returns an error, fix the value using the example in the error and call it again until it succeeds — do not call agent_finished while your finding is unsaved. After it succeeds, call agent_finished; if you cannot produce a valid value, call agent_fail with the reason.'),
       updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
 WHERE project_id = '__global__' AND workflow_id = 'deep-research';

-- 2. scope: allow 1-6 angles instead of a rigid 5, so narrow/closed questions don't
--    dead-end (Run A: a closed question could not produce 3 angles -> empty cascade).
UPDATE agent_definitions
   SET prompt = REPLACE(prompt,
       'Decompose it into 5 complementary web-search angles',
       'Decompose it into 1-6 complementary web-search angles (a narrow or closed question may need only 1-2)')
 WHERE project_id = '__global__' AND workflow_id = 'deep-research' AND id = 'scope';
UPDATE agent_definitions
   SET prompt = REPLACE(prompt, '(3-6 angles).', '(1-6 angles).')
 WHERE project_id = '__global__' AND workflow_id = 'deep-research' AND id = 'scope';

-- 3. angles schema floor 3 -> 1 to match the relaxed scope prompt (the only minItems
--    in the bundled schemas; verified single occurrence).
UPDATE workflows
   SET finding_schemas = REPLACE(finding_schemas, '"minItems":3', '"minItems":1'),
       updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
 WHERE project_id = '__global__' AND id = 'deep-research';

-- 4. Verifiers: grant web_fetch + artifact access so they reuse the researcher's
--    cached pages instead of re-fetching (the L2 wall-clock bottleneck in Run B).
UPDATE agent_definitions
   SET tools = 'web_search,web_fetch,artifact_list,artifact_get,emit_findings',
       updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
 WHERE project_id = '__global__' AND workflow_id = 'deep-research'
   AND id IN ('verify_a', 'verify_b', 'verify_c');

-- 5. Verifier source-access guidance: prefer the researcher's artifacts, use only the
--    nrflo web tools (not native browsing), never re-fetch/re-search the same URL/query.
UPDATE agent_definitions
   SET prompt = REPLACE(prompt,
       'Reference each claim by its 0-based index in the claims array above.',
       'You have artifact_list/artifact_get to read pages the researcher already fetched (named websrc_*), plus web_search/web_fetch to gather more. Prefer reading an already-fetched page over re-fetching it; use only these nrflo tools (not any built-in web-browsing tool); and never fetch or search the same URL or query twice — record your verdict and move on. Reference each claim by its 0-based index in the claims array above.')
 WHERE project_id = '__global__' AND workflow_id = 'deep-research'
   AND id IN ('verify_a', 'verify_b', 'verify_c')
   -- idempotency guard: the replacement re-contains the anchor sentence, so skip
   -- rows that already carry the guidance (REPLACE alone would double it).
   AND prompt NOT LIKE '%artifact_list/artifact_get to read pages the researcher already fetched%';

-- 6. research: cap claims to the ~20 most central to bound L2 verification cost.
UPDATE agent_definitions
   SET prompt = REPLACE(prompt,
       'A failed/blocked fetch returns ok:false — note it and move on; do not invent content. Emit one finding with emit_findings, key "claims" (the array of all extracted claims).',
       'A failed/blocked fetch returns ok:false — note it and move on; do not invent content. Prioritize the most central, important claims and emit at most ~20 in total (drop tangential ones beyond that). Emit one finding with emit_findings, key "claims" (the array of all extracted claims).')
 WHERE project_id = '__global__' AND workflow_id = 'deep-research' AND id = 'research';
