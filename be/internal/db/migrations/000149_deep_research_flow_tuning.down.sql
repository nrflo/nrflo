-- Revert the deep-research flow tuning (see the up migration).
UPDATE agent_definitions
   SET prompt = REPLACE(prompt,
       'A failed/blocked fetch returns ok:false — note it and move on; do not invent content. Prioritize the most central, important claims and emit at most ~20 in total (drop tangential ones beyond that). Emit one finding with emit_findings, key "claims" (the array of all extracted claims).',
       'A failed/blocked fetch returns ok:false — note it and move on; do not invent content. Emit one finding with emit_findings, key "claims" (the array of all extracted claims).')
 WHERE project_id = '__global__' AND workflow_id = 'deep-research' AND id = 'research';

UPDATE agent_definitions
   SET prompt = REPLACE(prompt,
       'You have artifact_list/artifact_get to read pages the researcher already fetched (named websrc_*), plus web_search/web_fetch to gather more. Prefer reading an already-fetched page over re-fetching it; use only these nrflo tools (not any built-in web-browsing tool); and never fetch or search the same URL or query twice — record your verdict and move on. Reference each claim by its 0-based index in the claims array above.',
       'Reference each claim by its 0-based index in the claims array above.')
 WHERE project_id = '__global__' AND workflow_id = 'deep-research'
   AND id IN ('verify_a', 'verify_b', 'verify_c');

UPDATE agent_definitions
   SET tools = 'web_search,emit_findings'
 WHERE project_id = '__global__' AND workflow_id = 'deep-research'
   AND id IN ('verify_a', 'verify_b', 'verify_c');

UPDATE workflows
   SET finding_schemas = REPLACE(finding_schemas, '"minItems":1', '"minItems":3')
 WHERE project_id = '__global__' AND id = 'deep-research';

UPDATE agent_definitions
   SET prompt = REPLACE(prompt, '(1-6 angles).', '(3-6 angles).')
 WHERE project_id = '__global__' AND workflow_id = 'deep-research' AND id = 'scope';
UPDATE agent_definitions
   SET prompt = REPLACE(prompt,
       'Decompose it into 1-6 complementary web-search angles (a narrow or closed question may need only 1-2)',
       'Decompose it into 5 complementary web-search angles')
 WHERE project_id = '__global__' AND workflow_id = 'deep-research' AND id = 'scope';

UPDATE agent_definitions
   SET prompt = REPLACE(prompt,
       'If emit_findings returns an error, fix the value using the example in the error and call it again until it succeeds — do not call agent_finished while your finding is unsaved. After it succeeds, call agent_finished; if you cannot produce a valid value, call agent_fail with the reason.',
       'Then call agent_finished.')
 WHERE project_id = '__global__' AND workflow_id = 'deep-research';
