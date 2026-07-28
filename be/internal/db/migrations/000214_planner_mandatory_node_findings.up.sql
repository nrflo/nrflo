-- Two planner-prompt corrections. Fresh installs get them from the Go const
-- (dynamic_seed_planner.go); the system planners are migration-seeded only
-- (000158), so every row is updated here. Idempotent: each UPDATE is an
-- anchored REPLACE guarded by a NOT LIKE on the replacement text.
--
-- 1. Data flow was documented as optional ("A node may reference an earlier
--    layer's finding"), so planners named upstream nodes in prose instead of
--    interpolating them. A node only ever receives its own instructions, so a
--    prose reference resolves to nothing and the verify/synthesis layers run
--    blind on invented input. Observed on two consecutive real runs.
--
-- 2. The provider-diverse verify guidance told the planner to use "quorum:2"
--    "so the workflow tolerates one provider being unavailable" — but quorum:2
--    on a two-node layer requires BOTH nodes to pass, so a single provider
--    outage fails the run. The rationale and the policy contradicted each
--    other; "any" is what the stated intent actually needs.

UPDATE agent_definitions
   SET prompt = REPLACE(prompt,
       '- A node may reference an earlier layer''s finding with `#{NODE_FINDINGS:<node-id>}` inside its instructions — never reference a node in the same or a later layer.',
       '- DATA FLOW IS EXPLICIT AND MANDATORY. A node receives ONLY its own instructions. Naming an earlier node in prose ("read the research nodes'' findings", "verify the claims from layer 0") delivers NOTHING — that text is never resolved. Every node that consumes an earlier node''s output MUST inline `#{NODE_FINDINGS:<node-id>}` at the point it is needed, once per source node, referencing only strictly earlier layers — never the same or a later one. A verifier or synthesizer without these placeholders runs blind and invents its input, so before you emit: for each node after layer 0, check that every node it claims to read appears as a placeholder in its instructions.'),
       updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
 WHERE project_id = '__global__' AND workflow_id = 'dynamic' AND node_role = 'planner'
   AND prompt NOT LIKE '%DATA FLOW IS EXPLICIT AND MANDATORY%';

UPDATE system_agent_definitions
   SET prompt = REPLACE(prompt,
       '- A node may reference an earlier layer''s finding with `#{NODE_FINDINGS:<node-id>}` inside its instructions — never reference a node in the same or a later layer.',
       '- DATA FLOW IS EXPLICIT AND MANDATORY. A node receives ONLY its own instructions. Naming an earlier node in prose ("read the research nodes'' findings", "verify the claims from layer 0") delivers NOTHING — that text is never resolved. Every node that consumes an earlier node''s output MUST inline `#{NODE_FINDINGS:<node-id>}` at the point it is needed, once per source node, referencing only strictly earlier layers — never the same or a later one. A verifier or synthesizer without these placeholders runs blind and invents its input, so before you emit: for each node after layer 0, check that every node it claims to read appears as a placeholder in its instructions.'),
       updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
 WHERE role = 'planner'
   AND prompt NOT LIKE '%DATA FLOW IS EXPLICIT AND MANDATORY%';

UPDATE agent_definitions
   SET prompt = REPLACE(prompt,
       'prefer a provider-diverse verify layer (both bound in the same layer) with policy "quorum:2" so the workflow tolerates one provider being unavailable.',
       'prefer a provider-diverse verify layer (both bound in the same layer) with policy "any", so the workflow still advances when one provider is unavailable. Do not use "quorum:2" for a two-node layer — that requires BOTH to pass and makes a single provider outage fail the whole run.
- Diversity of MODEL is not diversity of MANDATE. Two verifiers handed the same scope share a blind spot no matter how skeptical their prompts, so give each verifier its own cluster of claims, and when the goal rests on assumptions the caller stated rather than tested, add a premise-auditor node in LAYER 0 — unanchored, alongside the research, never downstream of it.'),
       updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
 WHERE project_id = '__global__' AND workflow_id = 'dynamic' AND node_role = 'planner'
   AND prompt NOT LIKE '%Diversity of MODEL is not diversity of MANDATE%';
