-- Steer the dynamic-workflow planner (and the system planner shared by every
-- other plan-driven workflow) to cheap-tier workers by default, and set the
-- __global__/dynamic template catalog's cheap-tier reasoning_effort defaults.
-- Fresh installs get all of this from the Go const/seed (dynamic_seed_planner.go,
-- dynamic_seed_data.go); EnsureGlobalDynamicWorkflow is create-if-absent, so
-- existing installs need these UPDATEs. Mirrors the anchored REPLACE idiom in
-- 000151_deep_research_lean_verifiers.up.sql; idempotent (0 rows once the
-- anchor is consumed, or once reasoning_effort already matches).

UPDATE agent_definitions
   SET prompt = REPLACE(prompt,
       '- Never invent a template, model, tool, or finding key that is not in the library above. If the library is missing something you need, do not substitute a similar template silently — emit a question in questions[] describing the gap instead.

## Manifest Schema (version 1)',
       '- Never invent a template, model, tool, or finding key that is not in the library above. If the library is missing something you need, do not substitute a similar template silently — emit a question in questions[] describing the gap instead.

## Tier Policy

- Default every worker node to the cheap tier (haiku, low effort) unless the node genuinely needs deep reasoning to do its job — justify anything above sonnet-medium in the node''s instructions or the plan''s rationale.
- Fan-out/per-file/per-item sweep nodes are ALWAYS cheap tier — a wide fan-out of premium nodes is never justified by breadth alone.
- Reserve at most one synthesis/judge node at mid tier (sonnet) for merging or adjudicating other nodes'' output.
- Premium tier (opus/fable) is reserved for final adjudication only, and only when the caller''s task genuinely demands it — not a default choice.
- Verification nodes belong at sonnet-low and should be provider/perspective-diverse (see Delegation Doctrine above), never premium.
- Premium nodes are capped server-side (dynwf_max_premium_workers, default 2): a plan that binds more than the cap to a premium-tier template is rejected at interactive approval, or auto-downgraded to a cheaper template with a warning finding in unattended (mode=auto) runs. Stay under the cap.

## Manifest Schema (version 1)'),
       updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
 WHERE project_id = '__global__' AND workflow_id = 'dynamic' AND id = 'dynamic-planner';

UPDATE system_agent_definitions
   SET prompt = REPLACE(prompt,
       '## Previous Manifest (if replanning)

${PREVIOUS_MANIFEST}

## Manifest Schema (version 1)',
       '## Previous Manifest (if replanning)

${PREVIOUS_MANIFEST}

## Tier Policy

- Default every worker node to the cheap tier (haiku, low effort) unless the node genuinely needs deep reasoning to do its job — justify anything above sonnet-medium in the node''s instructions or the plan''s rationale.
- Fan-out/per-file/per-item sweep nodes are ALWAYS cheap tier — a wide fan-out of premium nodes is never justified by breadth alone.
- Reserve at most one synthesis/judge node at mid tier (sonnet) for merging or adjudicating other nodes'' output.
- Premium tier (opus/fable) is reserved for final adjudication only, and only when the caller''s task genuinely demands it — not a default choice.
- Verification nodes belong at sonnet-low and should be provider/perspective-diverse, never premium.
- Premium nodes are capped server-side (dynwf_max_premium_workers, default 2): a plan that binds more than the cap to a premium-tier template is rejected at interactive approval, or auto-downgraded to a cheaper template with a warning finding in unattended (mode=auto) runs. Stay under the cap.

## Manifest Schema (version 1)'),
       updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
 WHERE id IN ('planner-system', 'planner-system-api');

UPDATE agent_definitions
   SET reasoning_effort = 'low',
       updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
 WHERE project_id = '__global__' AND workflow_id = 'dynamic'
   AND id IN ('codebase-explorer', 'module-reviewer', 'implementor-worker',
              'web-researcher', 'finding-verifier', 'generic-worker', 'cross-checker')
   AND (reasoning_effort IS NULL OR reasoning_effort <> 'low');

UPDATE agent_definitions
   SET reasoning_effort = 'medium',
       updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
 WHERE project_id = '__global__' AND workflow_id = 'dynamic' AND id = 'synthesizer'
   AND (reasoning_effort IS NULL OR reasoning_effort <> 'medium');
