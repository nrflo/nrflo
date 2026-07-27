-- Add two templates to the __global__/dynamic catalog and teach the verifiers
-- to check relevance, not just sourcing. Fresh installs get all of this from
-- the Go seed (dynamic_seed_data.go, dynamic_seed_prompts_verify.go,
-- dynamic_seed_schemas.go); EnsureGlobalDynamicWorkflow returns early once the
-- workflow row exists, so existing installs need these statements. Idempotent:
-- INSERT OR IGNORE on the (project_id, workflow_id, id) PK, and anchored
-- REPLACEs that match nothing once consumed.
--
-- web-researcher-cheap closes a real gap: 000175 told the planner to default
-- workers to cheap tier, but the catalog had no cheap research template, so
-- every web-research fan-out was forced to sonnet. Its prompt/tools are copied
-- from the sonnet row so the two stay contract-identical by construction.

INSERT OR IGNORE INTO agent_definitions
    (id, project_id, workflow_id, model, timeout, prompt, layer, execution_mode,
     tools, node_role, description, reasoning_effort, created_at, updated_at)
SELECT 'web-researcher-cheap', project_id, workflow_id, 'haiku-4-5', timeout, prompt,
       layer, execution_mode, tools, node_role,
       'Cheap-tier twin of web-researcher (haiku): identical claim-extraction contract at a fraction of the cost. The default binding for wide research fan-outs — reach for web-researcher only when a node needs deeper reasoning over what it reads. Emits to finding key `claims`.',
       'low',
       strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
  FROM agent_definitions
 WHERE project_id = '__global__' AND workflow_id = 'dynamic' AND id = 'web-researcher';

INSERT OR IGNORE INTO agent_definitions
    (id, project_id, workflow_id, model, timeout, prompt, layer, execution_mode,
     tools, node_role, description, reasoning_effort, created_at, updated_at)
SELECT 'premise-auditor', project_id, workflow_id, 'sonnet-5', timeout,
'You are a premise-auditor node in a dynamically planned workflow. You do not evaluate the proposal and you do not check other nodes'' claims — you attack the question itself. Your instructions for this node:
${NODE_INSTRUCTIONS}

List every assumption the goal treats as settled but never tests, including ones inherited from the caller''s own phrasing — those are the easiest to miss, because every downstream node repeats them faithfully. For each, name the one measurement or source that would falsify it and who would have to be wrong. Rank by whether being wrong changes the conclusion, not by whether it changes a number: a comparison baseline nobody checked is worth more than a rounding error. Use web_search only to judge whether a falsifier is cheap to obtain — do not run the wider research yourself, other nodes own that.

Emit one finding with emit_findings, key "premises", value the array of {premise, status ("tested"|"untested"|"contradicted"), falsifier, impact ("decisive"|"material"|"minor")}. If emit_findings returns an error, fix the value using the example in the error and call it again until it succeeds — do not call agent_finished while your finding is unsaved. After it succeeds, call agent_finished; if you cannot produce a valid value, call agent_fail with the reason.',
       layer, execution_mode, 'web_search,web_fetch,emit_findings', node_role,
       'Attacks the goal itself rather than the findings: names assumptions the plan treats as settled but never tests, and the measurement that would falsify each. Bind it in the FIRST layer alongside the research nodes, never after them, so it is not anchored by their output. Emits to finding key `premises`.',
       'low',
       strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
  FROM agent_definitions
 WHERE project_id = '__global__' AND workflow_id = 'dynamic' AND id = 'finding-verifier';

-- premise-auditor emits to a new key; FindingsService.Emit hard-rejects any key
-- with no declared schema, so the workflow's finding_schemas must carry it.
UPDATE workflows
   SET finding_schemas = REPLACE(finding_schemas,
       '{"key":"cross_check","schema":',
       '{"key":"premises","schema":{"type":"array","items":{"type":"object","required":["premise","status","falsifier","impact"],"properties":{"premise":{"type":"string"},"status":{"enum":["tested","untested","contradicted"]},"falsifier":{"type":"string"},"impact":{"enum":["decisive","material","minor"]}}}},"example":[]},
{"key":"cross_check","schema":'),
       updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
 WHERE project_id = '__global__' AND id = 'dynamic'
   AND finding_schemas NOT LIKE '%"key":"premises"%';

-- Verifiers previously checked claim -> source only. A claim can be correctly
-- sourced and arithmetically right and still be a category error (a comparison
-- against a capability nothing in the system can reach). Make achievability
-- part of the verdict.
UPDATE agent_definitions
   SET prompt = REPLACE(prompt,
       'You may web_search to check specific facts; keep your context lean by searching narrowly rather than re-reading everything.',
       'You may web_search to check specific facts; keep your context lean by searching narrowly rather than re-reading everything.

Check relevance as well as truth. A claim can be correctly sourced and arithmetically right and still be a category error: a comparison against a capability nothing in the system can actually reach, a rate some other component caps below the quoted figure, or a number that measures a different quantity than the argument needs. Verify that each decisive claim is achievable and on-topic, not merely cited — and REFUTE it when it is not, saying which constraint it collides with.'),
       updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
 WHERE project_id = '__global__' AND workflow_id = 'dynamic'
   AND id IN ('finding-verifier', 'finding-verifier-codex')
   AND prompt NOT LIKE '%Check relevance as well as truth%';
