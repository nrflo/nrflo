-- deep-research scope agent can be grounded in caller-supplied context: the
-- mcp-external deep_research tool passes an optional `context` arg as the
-- instance's external_context, surfaced in the scope prompt via
-- ${EXTERNAL_CONTEXT}. Opt-in: when no context is supplied the injected section
-- is blank and the agent ignores it. Bring already-seeded installs in line with
-- the seed (fresh installs get this from the seed; 0 rows otherwise). The
-- NOT LIKE guard keeps it idempotent (the replacement re-contains the anchor).
UPDATE agent_definitions
   SET prompt = REPLACE(prompt,
       'The user''s research question is provided above as your instructions.',
       'The user''s research question is provided above as your instructions.

Caller-supplied context (may be empty — if blank, ignore it and research the question on its own terms):
${EXTERNAL_CONTEXT}

When that context is present, bias the angles toward what it implies the caller cares about (their domain, tech stack and versions, constraints, and what they already know) while still covering the question objectively.'),
       updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
 WHERE project_id = '__global__' AND workflow_id = 'deep-research' AND id = 'scope'
   AND prompt NOT LIKE '%Caller-supplied context%';
